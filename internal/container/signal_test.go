package container

import (
	"os/exec"
	"testing"
	"time"

	"minicontainer/internal/state"
)

func TestSendSignal(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child process: %v", err)
	}
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	tmpDir := t.TempDir()
	st, err := state.Open(tmpDir)
	if err != nil {
		t.Fatalf("Open state store error: %v", err)
	}

	c := &state.Container{
		ID:        "ctr-sig-1",
		Status:    state.StatusRunning,
		PID:       cmd.Process.Pid,
		CreatedAt: time.Now(),
	}
	if err := st.Save(c); err != nil {
		t.Fatalf("save container state: %v", err)
	}

	if err := SendSignal(st, c.ID, "SIGHUP"); err != nil {
		t.Fatalf("SendSignal error: %v", err)
	}
}
