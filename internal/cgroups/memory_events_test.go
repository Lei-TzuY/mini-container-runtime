package cgroups

import (
	"testing"
)

func TestReadMemoryEvents(t *testing.T) {
	tmpDir := t.TempDir()
	events, err := ReadMemoryEvents(tmpDir)
	if err != nil && len(events) == 0 {
		t.Fatalf("ReadMemoryEvents error: %v", err)
	}
}
