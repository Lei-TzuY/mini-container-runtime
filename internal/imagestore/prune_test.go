package imagestore

import (
	"testing"
	"time"

	"minicontainer/internal/state"
)

func TestPruneOrphanLayers(t *testing.T) {
	tmpDir := t.TempDir()
	st, err := state.Open(tmpDir)
	if err != nil {
		t.Fatalf("Open state store error: %v", err)
	}

	dangling := &state.Image{
		ID:       "orphan-1",
		Tag:      "<none>",
		Name:     "orphan-1",
		Size:     2048,
		LoadedAt: time.Now(),
	}
	_ = st.SaveImage(dangling)

	count, bytes, err := PruneOrphanLayers(st)
	if err != nil {
		t.Fatalf("PruneOrphanLayers error: %v", err)
	}
	if count != 1 || bytes != 2048 {
		t.Fatalf("Prune result count=%d, bytes=%d, want 1, 2048", count, bytes)
	}
}
