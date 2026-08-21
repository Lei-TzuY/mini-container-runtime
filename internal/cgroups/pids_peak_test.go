package cgroups

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestReadPIDSPeak_Missing(t *testing.T) {
	tmpDir := t.TempDir()
	peak, err := ReadPIDSPeak(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if peak != 0 {
		t.Errorf("expected 0 for missing file, got %d", peak)
	}
}

func TestWriteAndReadPIDSPeak_Linux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("cgroup file writing only works on Linux")
	}

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "pids.peak"), []byte("256\n"), 0644); err != nil {
		t.Fatal(err)
	}

	val, err := ReadPIDSPeak(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != 256 {
		t.Errorf("val = %d, want 256", val)
	}

	if err := ResetPIDSPeak(tmpDir); err != nil {
		t.Fatalf("ResetPIDSPeak failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "pids.peak"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "0\n" {
		t.Errorf("file data = %q, want '0\\n'", string(data))
	}
}
