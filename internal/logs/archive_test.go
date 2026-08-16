package logs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestArchiveLogFile(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "container.log")
	_ = os.WriteFile(logPath, []byte("content"), 0644)

	if err := ArchiveLogFile(logPath, 3); err != nil {
		t.Fatalf("ArchiveLogFile error: %v", err)
	}

	if _, err := os.Stat(logPath + ".1"); err != nil {
		t.Fatalf("Archived log file container.log.1 does not exist: %v", err)
	}
}
