package logs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompressRotatedLogRejectsGroupWritableDestinationBeforeTruncate(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "container.log.1")
	gzPath := logPath + ".gz"

	if err := os.WriteFile(logPath, []byte("new log data\n"), 0644); err != nil {
		t.Fatal(err)
	}
	const existing = "existing archive\n"
	if err := os.WriteFile(gzPath, []byte(existing), 0660); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(gzPath, 0660); err != nil {
		t.Fatal(err)
	}

	if err := CompressRotatedLog(logPath); err == nil {
		t.Fatal("expected group-writable gzip destination to be rejected")
	}

	got, err := os.ReadFile(gzPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != existing {
		t.Fatalf("gzip destination was modified: got %q, want %q", got, existing)
	}
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("source log should be retained: %v", err)
	}
}
