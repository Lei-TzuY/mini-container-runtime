package container

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"minicontainer/internal/state"
)

func TestCommitContainer(t *testing.T) {
	tmpDir := t.TempDir()
	st, err := state.Open(tmpDir)
	if err != nil {
		t.Fatalf("Open state store error: %v", err)
	}

	rootFS := filepath.Join(tmpDir, "rootfs")
	_ = os.MkdirAll(rootFS, 0755)
	_ = os.WriteFile(filepath.Join(rootFS, "app.txt"), []byte("committed app data"), 0644)

	c := &state.Container{
		ID:        "ctr-cmt-1",
		Status:    state.StatusStopped,
		RootFS:    rootFS,
		CreatedAt: time.Now(),
	}
	_ = st.Save(c)

	img, err := CommitContainer(st, c.ID, "mycommit:v1")
	if err != nil {
		t.Fatalf("CommitContainer error: %v", err)
	}

	if img.Tag != "mycommit:v1" {
		t.Fatalf("Image Tag = %s, want mycommit:v1", img.Tag)
	}
}
