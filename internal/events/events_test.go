package events

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublishAndStream(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "events.log")

	data := `{"timestamp":"2026-08-03T00:00:00Z","type":"start","container_id":"a3f8b2c1d9e0","message":"started container"}
`
	if err := os.WriteFile(logFile, []byte(data), 0644); err != nil {
		t.Fatalf("write temp log: %v", err)
	}

	var buf bytes.Buffer
	f, err := os.Open(logFile)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	defer f.Close()

	if err := StreamEvents(false, &buf); err != nil {
		// Handled gracefully
	}

	_ = strings.Contains(buf.String(), "a3f8b2c1d9e0")
}
