package cgroups

import (
	"testing"
)

func TestReadPSI(t *testing.T) {
	tmpDir := t.TempDir()
	psi, err := ReadPSI(tmpDir, "memory")
	if err != nil {
		t.Fatalf("ReadPSI error: %v", err)
	}
	if psi == nil {
		t.Fatalf("ReadPSI returned nil struct")
	}
}
