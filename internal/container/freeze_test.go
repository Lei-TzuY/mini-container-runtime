package container

import (
	"testing"
	"time"

	"minicontainer/internal/state"
)

func TestFreezeThawContainer(t *testing.T) {
	tmpDir := t.TempDir()
	st, err := state.Open(tmpDir)
	if err != nil {
		t.Fatalf("Open state store error: %v", err)
	}

	c := &state.Container{
		ID:        "ctr-frz-1",
		Status:    state.StatusRunning,
		PID:       9999,
		CreatedAt: time.Now(),
	}
	_ = st.Save(c)

	_ = FreezeContainer(st, c.ID)
	_ = ThawContainer(st, c.ID)
}
