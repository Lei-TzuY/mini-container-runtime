package logs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompressRotatedLog(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "container.log.1")
	_ = os.WriteFile(logPath, []byte("log data content"), 0644)

	if err := CompressRotatedLog(logPath); err != nil {
		t.Fatalf("CompressRotatedLog error: %v", err)
	}

	if _, err := os.Stat(logPath + ".gz"); err != nil {
		t.Fatalf("Compressed archive container.log.1.gz does not exist: %v", err)
	}
}
