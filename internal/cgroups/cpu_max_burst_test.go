package cgroups

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestReadCPUMaxBurst_Missing(t *testing.T) {
	tmpDir := t.TempDir()
	burst, err := ReadCPUMaxBurst(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if burst != 0 {
		t.Errorf("expected 0 for missing file, got %d", burst)
	}
}

func TestWriteAndReadCPUMaxBurst_Linux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("cgroup file writing only works on Linux")
	}

	tmpDir := t.TempDir()
	if err := WriteCPUMaxBurst(tmpDir, 50000); err != nil {
		t.Fatal(err)
	}

	val, err := ReadCPUMaxBurst(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != 50000 {
		t.Errorf("val = %d, want 50000", val)
	}

	// Verify file content
	data, err := os.ReadFile(filepath.Join(tmpDir, "cpu.max.burst"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "50000\n" {
		t.Errorf("file data = %q, want '50000\\n'", string(data))
	}
}
