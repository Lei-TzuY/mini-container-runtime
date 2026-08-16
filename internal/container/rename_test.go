package container

import (
	"testing"
	"time"

	"minicontainer/internal/state"
)

func TestRenameContainer(t *testing.T) {
	tmpDir := t.TempDir()
	st, err := state.Open(tmpDir)
	if err != nil {
		t.Fatalf("Open state store error: %v", err)
	}

	c := &state.Container{
		ID:        "ctr-ren-1",
		Hostname:  "old-name",
		Status:    state.StatusStopped,
		CreatedAt: time.Now(),
	}
	_ = st.Save(c)

	if err := RenameContainer(st, c.ID, "new-app-name"); err != nil {
		t.Fatalf("RenameContainer error: %v", err)
	}

	updated, err := st.Resolve(c.ID)
	if err != nil || updated.Hostname != "new-app-name" {
		t.Fatalf("Hostname = %s, want new-app-name", updated.Hostname)
	}
}
