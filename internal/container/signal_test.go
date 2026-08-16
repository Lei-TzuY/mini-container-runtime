package container

import (
	"os"
	"testing"
	"time"

	"minicontainer/internal/state"
)

func TestSendSignal(t *testing.T) {
	tmpDir := t.TempDir()
	st, err := state.Open(tmpDir)
	if err != nil {
		t.Fatalf("Open state store error: %v", err)
	}

	c := &state.Container{
		ID:        "ctr-sig-1",
		Status:    state.StatusRunning,
		PID:       os.Getpid(),
		CreatedAt: time.Now(),
	}
	_ = st.Save(c)

	if err := SendSignal(st, c.ID, "SIGHUP"); err != nil {
		t.Fatalf("SendSignal error: %v", err)
	}
}
