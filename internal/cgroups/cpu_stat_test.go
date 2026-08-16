package cgroups

import (
	"testing"
)

func TestReadCPUStat(t *testing.T) {
	tmpDir := t.TempDir()
	metrics, err := ReadCPUStat(tmpDir)
	if err != nil && len(metrics) == 0 {
		t.Fatalf("ReadCPUStat error: %v", err)
	}
}
