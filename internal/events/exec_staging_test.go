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
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		stagedExecs = make(map[string]stagedExecEvent)
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

func TestDiscardPendingExecSuppressesFailedSetup(t *testing.T) {
	resetExecStagingForTest(t)
	t.Setenv("HOME", t.TempDir())

	if err := Publish(EventExec, "ctr-fail", "rootfs", "exec [missing]"); err != nil {
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
