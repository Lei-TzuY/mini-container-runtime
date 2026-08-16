package container

import (
	"testing"
	"time"

	"minicontainer/internal/state"
)

func TestExecDetached(t *testing.T) {
	tmpDir := t.TempDir()
	st, err := state.Open(tmpDir)
	if err != nil {
		t.Fatalf("Open state store error: %v", err)
	}

	c := &state.Container{
		ID:        "ctr-det-1",
		Status:    state.StatusRunning,
		PID:       1234,
		CreatedAt: time.Now(),
	}
	_ = st.Save(c)

	pid, err := ExecDetached(st, c.ID, []string{"echo", "hello"})
	if err != nil {
		t.Fatalf("ExecDetached error: %v", err)
	}
	if pid == 0 {
		t.Fatalf("Returned pid is 0")
	}
}
