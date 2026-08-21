package cgroups

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReclaimMemoryWithOptions_Linux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("cgroup file writing only works on Linux")
	}

	tmpDir := t.TempDir()
	opts := MemoryReclaimOptions{
		BytesToReclaim: 2097152,
		Swappiness:     0,
		NumaNode:       1,
	}

	if err := ReclaimMemoryWithOptions(tmpDir, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "memory.reclaim"))
	if err != nil {
		t.Fatal(err)
	}

	content := strings.TrimSpace(string(data))
	expected := "2097152 swappiness=0 node=1"
	if content != expected {
		t.Errorf("got %q, want %q", content, expected)
	}
}

func TestReclaimMemoryWithOptions_Defaults(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("cgroup file writing only works on Linux")
	}

	tmpDir := t.TempDir()
	opts := MemoryReclaimOptions{
		BytesToReclaim: 0,
		Swappiness:     -1,
		NumaNode:       -1,
	}

	if err := ReclaimMemoryWithOptions(tmpDir, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "memory.reclaim"))
	if err != nil {
		t.Fatal(err)
	}

	content := strings.TrimSpace(string(data))
	if content != "1048576" {
		t.Errorf("got %q, want '1048576'", content)
	}
}
