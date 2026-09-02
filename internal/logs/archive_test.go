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

func TestArchiveLogFileReportsActiveRenameFailure(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "container.log")
	if err := os.WriteFile(logPath, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(logPath+".1", 0755); err != nil {
		t.Fatal(err)
	}

	if err := ArchiveLogFile(logPath, 3); err == nil {
		t.Fatal("expected archive failure when destination is a directory")
	}

	got, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("active log should remain after failed rename: %v", err)
	}
	if string(got) != "content" {
		t.Fatalf("active log changed after failed rename: %q", got)
	}
}
