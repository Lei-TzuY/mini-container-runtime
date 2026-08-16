package cgroups

import (
	"testing"
)

func TestReadOOMEvents(t *testing.T) {
	tmpDir := t.TempDir()
	evts, err := ReadOOMEvents(tmpDir)
	if err != nil {
		t.Fatalf("ReadOOMEvents error: %v", err)
	}
	if evts == nil {
		t.Fatalf("ReadOOMEvents returned nil struct")
	}
}
