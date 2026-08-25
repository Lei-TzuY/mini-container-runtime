//go:build linux

package container

import (
	"errors"
	"os/exec"
	"testing"
	"time"

	"minicontainer/internal/state"
)

func saveRunningTestContainer(t *testing.T, st *state.Store, id string, cmd *exec.Cmd, start uint64) {
	t.Helper()
	if err := st.Save(&state.Container{
		ID:           id,
		Status:       state.StatusRunning,
		PID:          cmd.Process.Pid,
		PIDStartTime: start,
		CreatedAt:    time.Now(),
	}); err != nil {
		t.Fatalf("save running container: %v", err)
	}
}

func TestStopContainerTerminatesExactProcessAndReconcilesState(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	defer func() {
		if cmd.Process != nil && IsRunning(cmd.Process.Pid) {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	start, err := ProcessStartTime(cmd.Process.Pid)
	if err != nil {
		t.Fatalf("ProcessStartTime: %v", err)
	}
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	saveRunningTestContainer(t, st, "ctr-stop-exact", cmd, start)

	if _, err := StopContainer(st, "ctr-stop-exact", time.Second); err != nil {
		t.Fatalf("StopContainer: %v", err)
	}
	if IsRunning(cmd.Process.Pid) {
		t.Fatalf("process %d still running after StopContainer", cmd.Process.Pid)
	}

	rec, err := st.Get("ctr-stop-exact")
	if err != nil {
		t.Fatalf("reload state: %v", err)
	}
	if rec.Status != state.StatusStopped || rec.PID != 0 || rec.PIDStartTime != 0 {
		t.Fatalf("state not reconciled: status=%s pid=%d start=%d", rec.Status, rec.PID, rec.PIDStartTime)
	}
	if rec.FinishedAt == nil || rec.ExitCode != -1 {
		t.Fatalf("stop metadata not recorded: finished=%v exit=%d", rec.FinishedAt, rec.ExitCode)
	}
}

func TestStopContainerRejectsStaleIdentityWithoutSignaling(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	start, err := ProcessStartTime(cmd.Process.Pid)
	if err != nil {
		t.Fatalf("ProcessStartTime: %v", err)
	}
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	saveRunningTestContainer(t, st, "ctr-stop-stale", cmd, start+1)

	_, err = StopContainer(st, "ctr-stop-stale", 10*time.Millisecond)
	if !errors.Is(err, ErrProcessIdentityMismatch) {
		t.Fatalf("expected identity mismatch, got %v", err)
	}
	match, err := ProcessIdentityMatches(cmd.Process.Pid, start)
	if err != nil || !match {
		t.Fatalf("stale stop affected live process: match=%v err=%v", match, err)
	}
}

func TestStopContainerRejectsMissingIdentity(t *testing.T) {
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Save(&state.Container{
		ID:        "ctr-stop-legacy",
		Status:    state.StatusRunning,
		PID:       1234,
		CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := StopContainer(st, "ctr-stop-legacy", time.Second); !errors.Is(err, ErrProcessIdentityUnavailable) {
		t.Fatalf("expected missing identity error, got %v", err)
	}
}

func TestSendSignalLeavesLifecycleStateRunning(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()
	start, err := ProcessStartTime(cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	saveRunningTestContainer(t, st, "ctr-signal-state", cmd, start)

	if err := SendSignal(st, "ctr-signal-state", "SIGCONT"); err != nil {
		t.Fatalf("SendSignal: %v", err)
	}
	rec, err := st.Get("ctr-signal-state")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != state.StatusRunning || rec.PID != cmd.Process.Pid || rec.PIDStartTime != start {
		t.Fatalf("non-terminating signal mutated lifecycle state: %+v", rec)
	}
}
