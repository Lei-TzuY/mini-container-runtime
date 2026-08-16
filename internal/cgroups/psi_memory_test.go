package cgroups

import (
	"testing"
)

func TestReadMemoryPSI(t *testing.T) {
	tmpDir := t.TempDir()
	psi, err := ReadMemoryPSI(tmpDir)
	if err != nil && psi == "" {
		t.Fatalf("ReadMemoryPSI error: %v", err)
	}
}
