package cgroups

import (
	"testing"
)

func TestReadMemoryEventsLocal(t *testing.T) {
	tmpDir := t.TempDir()
	events, err := ReadMemoryEventsLocal(tmpDir)
	if err != nil && len(events) == 0 {
		t.Fatalf("ReadMemoryEventsLocal error: %v", err)
	}
}
