package events

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func resetExecStagingForTest(t *testing.T) {
	t.Helper()
	mu.Lock()
	stagedExecs = make(map[string]stagedExecEvent)
	activeExecs = make(map[string]stagedExecEvent)
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		stagedExecs = make(map[string]stagedExecEvent)
		activeExecs = make(map[string]stagedExecEvent)
		mu.Unlock()
	})
}

func TestExecEventIsStagedUntilPayloadStartCommit(t *testing.T) {
	resetExecStagingForTest(t)
	t.Setenv("HOME", t.TempDir())

	if err := Publish(EventExec, "ctr-exec", "rootfs", "exec [true]"); err != nil {
		t.Fatalf("stage exec: %v", err)
	}
	if _, err := os.Stat(LogPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("exec event log exists before payload proof: err=%v", err)
	}

	commitFloor := time.Now()
	if err := CommitPendingExec(); err != nil {
		t.Fatalf("commit exec: %v", err)
	}
	got := readLifecycleEventsForTest(t)
	if len(got) != 1 || got[0].Type != EventExec || got[0].ContainerID != "ctr-exec" {
		t.Fatalf("events=%+v, want one committed exec", got)
	}
	if got[0].Timestamp.Before(commitFloor) {
		t.Fatalf("exec timestamp=%v predates payload-start floor=%v", got[0].Timestamp, commitFloor)
	}
}

func TestCommitPendingExecRetainsStagedAttributionWhenDurableAppendFails(t *testing.T) {
	resetExecStagingForTest(t)
	t.Setenv("HOME", t.TempDir())

	if err := Publish(EventExec, "ctr-retry", "rootfs", "exec [true]"); err != nil {
		t.Fatal(err)
	}
	// Block the append path after staging. Opening a directory as events.log is
	// reliably invalid even for privileged test runners, unlike chmod-based
	// permission failures.
	if err := os.Mkdir(LogPath(), 0o700); err != nil {
		t.Fatalf("block event log path: %v", err)
	}
	if err := CommitPendingExec(); err == nil {
		t.Fatal("commit unexpectedly succeeded with events.log as a directory")
	}

	mu.Lock()
	_, stillStaged := stagedExecs["ctr-retry"]
	_, becameActive := activeExecs["ctr-retry"]
	mu.Unlock()
	if !stillStaged || becameActive {
		t.Fatalf("append failure changed attribution: staged=%v active=%v", stillStaged, becameActive)
	}

	if err := os.Remove(LogPath()); err != nil {
		t.Fatalf("remove append blocker: %v", err)
	}
	if err := CommitPendingExec(); err != nil {
		t.Fatalf("retry durable exec commit: %v", err)
	}
	got := readLifecycleEventsForTest(t)
	if len(got) != 1 || got[0].Type != EventExec || got[0].ContainerID != "ctr-retry" {
		t.Fatalf("events after retry=%+v, want one durable exec start", got)
	}
}

func TestCompletePendingExecRecordsExactlyOneTerminalOutcome(t *testing.T) {
	resetExecStagingForTest(t)
	t.Setenv("HOME", t.TempDir())

	if err := Publish(EventExec, "ctr-exit", "rootfs", "exec [sh -c false]"); err != nil {
		t.Fatal(err)
	}
	if err := CommitPendingExec(); err != nil {
		t.Fatal(err)
	}
	if err := CompletePendingExec(17, ""); err != nil {
		t.Fatal(err)
	}
	// Completion consumes active attribution; a duplicate completion is a no-op
	// rather than duplicating a terminal record.
	if err := CompletePendingExec(99, "duplicate"); err != nil {
		t.Fatal(err)
	}

	got := readLifecycleEventsForTest(t)
	if len(got) != 2 || got[0].Type != EventExec || got[1].Type != EventExecExit {
		t.Fatalf("events=%+v, want exec then exec_exit", got)
	}
	if got[1].ContainerID != "ctr-exit" || !strings.Contains(got[1].Message, "exit_code=17") {
		t.Fatalf("terminal event=%+v", got[1])
	}
}

func TestFailPendingExecRecordsFailureWithoutStartedEvent(t *testing.T) {
	resetExecStagingForTest(t)
	t.Setenv("HOME", t.TempDir())

	if err := Publish(EventExec, "ctr-fail", "rootfs", "exec [missing]"); err != nil {
		t.Fatal(err)
	}
	if err := FailPendingExec("payload start not proven"); err != nil {
		t.Fatal(err)
	}

	got := readLifecycleEventsForTest(t)
	if len(got) != 1 || got[0].Type != EventExecFailed || got[0].ContainerID != "ctr-fail" {
		t.Fatalf("events=%+v, want one exec_failed", got)
	}
	if !strings.Contains(got[0].Message, "payload start not proven") {
		t.Fatalf("failure event lost cause: %+v", got[0])
	}
}

func TestDiscardPendingExecSuppressesFailedSetup(t *testing.T) {
	resetExecStagingForTest(t)
	t.Setenv("HOME", t.TempDir())

	if err := Publish(EventExec, "ctr-discard", "rootfs", "exec [missing]"); err != nil {
		t.Fatal(err)
	}
	if err := DiscardPendingExec(); err != nil {
		t.Fatalf("discard exec: %v", err)
	}
	if err := CommitPendingExec(); err != nil {
		t.Fatalf("commit after discard should be no-op: %v", err)
	}
	if _, err := os.Stat(LogPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("discarded exec wrote event log: err=%v", err)
	}
}

func TestCommitPendingExecRejectsAmbiguousContainers(t *testing.T) {
	resetExecStagingForTest(t)
	t.Setenv("HOME", t.TempDir())

	if err := Publish(EventExec, "ctr-a", "rootfs-a", "exec a"); err != nil {
		t.Fatal(err)
	}
	if err := Publish(EventExec, "ctr-b", "rootfs-b", "exec b"); err != nil {
		t.Fatal(err)
	}
	err := CommitPendingExec()
	if err == nil || !strings.Contains(err.Error(), "2 staged exec events") {
		t.Fatalf("ambiguous exec commit error=%v", err)
	}
	if _, statErr := os.Stat(LogPath()); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("ambiguous exec commit wrote event log: err=%v", statErr)
	}
}
