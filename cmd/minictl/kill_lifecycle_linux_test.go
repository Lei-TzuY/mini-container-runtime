//go:build linux

package main

import (
	"os/exec"
	"testing"
	"time"

	"minicontainer/internal/container"
	"minicontainer/internal/state"
)

func saveKillTestContainer(t *testing.T, st *state.Store, id string, cmd *exec.Cmd) {
	t.Helper()
	start, err := container.ProcessStartTime(cmd.Process.Pid)
	if err != nil {
		t.Fatalf("ProcessStartTime: %v", err)
	}
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

func TestKillContainerForOptionsSIGKILLFinalizesDurableState(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	defer func() {
		if cmd.Process != nil && container.IsRunning(cmd.Process.Pid) {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	saveKillTestContainer(t, st, "ctr-kill-finalize", cmd)

	resolved, err := killContainerForOptions(st, killCommandOptions{
		containerID: "ctr-kill-finalize",
		signal:      "SIGKILL",
	})
	if err != nil {
		t.Fatalf("killContainerForOptions: %v", err)
	}
	if resolved.ID != "ctr-kill-finalize" {
		t.Fatalf("resolved id=%q", resolved.ID)
	}
	if container.IsRunning(cmd.Process.Pid) {
		t.Fatalf("process %d still running after SIGKILL", cmd.Process.Pid)
	}

	rec, err := st.Get("ctr-kill-finalize")
	if err != nil {
		t.Fatalf("reload state: %v", err)
	}
	if rec.Status != state.StatusStopped || rec.PID != 0 || rec.PIDStartTime != 0 {
		t.Fatalf("state not finalized: status=%s pid=%d start=%d", rec.Status, rec.PID, rec.PIDStartTime)
	}
	if rec.FinishedAt == nil || rec.ExitCode != -1 {
		t.Fatalf("stop metadata missing: finished=%v exit=%d", rec.FinishedAt, rec.ExitCode)
	}
}

func TestKillContainerForOptionsNonTerminatingSignalDoesNotFinalize(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	saveKillTestContainer(t, st, "ctr-kill-sigcont", cmd)

	if _, err := killContainerForOptions(st, killCommandOptions{
		containerID: "ctr-kill-sigcont",
		signal:      "SIGCONT",
	}); err != nil {
		t.Fatalf("killContainerForOptions SIGCONT: %v", err)
	}

	rec, err := st.Get("ctr-kill-sigcont")
	if err != nil {
		t.Fatalf("reload state: %v", err)
	}
	if rec.Status != state.StatusRunning || rec.PID != cmd.Process.Pid || rec.PIDStartTime == 0 {
		t.Fatalf("non-terminating signal changed lifecycle state: status=%s pid=%d start=%d", rec.Status, rec.PID, rec.PIDStartTime)
	}
}
