package cgroups

import (
	"testing"
)

func TestReadIOStat(t *testing.T) {
	tmpDir := t.TempDir()
	metrics, err := ReadIOStat(tmpDir)
	if err != nil && len(metrics) == 0 {
		t.Fatalf("ReadIOStat error: %v", err)
	}
}
