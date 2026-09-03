package logs

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPruneRotatedLogsDoesNotDeleteReplacementAfterAgeCheck(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "container.log.2")
	originalPath := filepath.Join(tmpDir, "original.log.2")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(path, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	oldHook := pruneBeforeDelete
	pruneBeforeDelete = func(deletePath string) {
		if deletePath != path {
			return
		}
		if err := os.Rename(path, originalPath); err != nil {
			t.Fatalf("move inspected archive: %v", err)
		}
		if err := os.WriteFile(path, []byte("fresh replacement"), 0o644); err != nil {
			t.Fatalf("create replacement archive: %v", err)
		}
	}
	defer func() { pruneBeforeDelete = oldHook }()

	deleted, err := PruneRotatedLogs(tmpDir, time.Hour)
	if err == nil {
		t.Fatal("expected prune failure after candidate pathname replacement")
	}
	if deleted != 0 {
		t.Fatalf("deleted count = %d, want 0", deleted)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("replacement should remain: %v", err)
	}
	if string(got) != "fresh replacement" {
		t.Fatalf("replacement changed: %q", got)
	}
	got, err = os.ReadFile(originalPath)
	if err != nil {
		t.Fatalf("original inspected archive should remain: %v", err)
	}
	if string(got) != "old" {
		t.Fatalf("original archive changed: %q", got)
	}
}
