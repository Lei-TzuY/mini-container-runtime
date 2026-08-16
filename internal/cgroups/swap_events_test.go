package cgroups

import (
	"testing"
)

func TestReadSwapEvents(t *testing.T) {
	tmpDir := t.TempDir()
	events, err := ReadSwapEvents(tmpDir)
	if err != nil && len(events) == 0 {
		t.Fatalf("ReadSwapEvents error: %v", err)
	}
}
