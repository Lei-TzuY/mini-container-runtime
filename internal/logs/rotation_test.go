package logs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRotateLogFile(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "container.log")
	_ = os.WriteFile(logPath, []byte("12345678901234567890"), 0644)

	if err := RotateLogFile(logPath, 10); err != nil {
		t.Fatalf("RotateLogFile error: %v", err)
	}

	data, _ := os.ReadFile(logPath)
	if len(data) != 10 {
		t.Fatalf("RotateLogFile size = %d, want 10", len(data))
	}
}
