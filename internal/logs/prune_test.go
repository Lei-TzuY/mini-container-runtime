package logs

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPruneRotatedLogs(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "container.log.1.gz")
	_ = os.WriteFile(logFile, []byte("old log"), 0644)

	count, err := PruneRotatedLogs(tmpDir, 1*time.Millisecond)
	if err != nil && count == 0 {
		t.Fatalf("PruneRotatedLogs error: %v", err)
	}
}
