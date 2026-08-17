package cgroups

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadPSI(t *testing.T) {
	tmpDir := t.TempDir()
	fixture := "some avg10=0.00 avg60=0.00 avg300=0.00 total=0\nfull avg10=0.00 avg60=0.00 avg300=0.00 total=0\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "memory.pressure"), []byte(fixture), 0o644); err != nil {
		t.Fatalf("write memory.pressure fixture: %v", err)
	}
	psi, err := ReadPSI(tmpDir, "memory")
	if err != nil {
		t.Fatalf("ReadPSI error: %v", err)
	}
	if psi == nil {
		t.Fatalf("ReadPSI returned nil struct")
	}
}
