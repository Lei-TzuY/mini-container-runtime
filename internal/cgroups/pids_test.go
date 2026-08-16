package cgroups

import (
	"testing"
)

func TestPIDsLimit(t *testing.T) {
	tmpDir := t.TempDir()
	if err := ApplyPIDsLimit(tmpDir, 100); err != nil {
		t.Fatalf("ApplyPIDsLimit error: %v", err)
	}

	cur, err := ReadPIDsCurrent(tmpDir)
	if err != nil {
		t.Fatalf("ReadPIDsCurrent error: %v", err)
	}
	if cur < 0 {
		t.Fatalf("Current PIDs = %d, want >= 0", cur)
	}
}
