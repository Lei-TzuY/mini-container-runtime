package cgroups

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestReadMemorySwapCurrent_Missing(t *testing.T) {
	tmpDir := t.TempDir()
	cur, err := ReadMemorySwapCurrent(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cur != 0 {
		t.Errorf("expected 0 for missing file, got %d", cur)
	}
}

func TestReadMemorySwapHigh_Missing(t *testing.T) {
	tmpDir := t.TempDir()
	high, isMax, err := ReadMemorySwapHigh(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isMax || high != 0 {
		t.Errorf("expected isMax=true and high=0 for missing file, got high=%d isMax=%t", high, isMax)
	}
}

func TestWriteAndReadMemorySwapHigh_Linux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("cgroup file writing only works on Linux")
	}

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "memory.swap.current"), []byte("1048576\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := WriteMemorySwapHigh(tmpDir, 20971520); err != nil {
		t.Fatal(err)
	}

	cur, err := ReadMemorySwapCurrent(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cur != 1048576 {
		t.Errorf("cur = %d, want 1048576", cur)
	}

	high, isMax, err := ReadMemorySwapHigh(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isMax || high != 20971520 {
		t.Errorf("high = %d isMax = %t, want 20971520 false", high, isMax)
	}

	if err := WriteMemorySwapHigh(tmpDir, -1); err != nil {
		t.Fatal(err)
	}
	high, isMax, err = ReadMemorySwapHigh(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isMax {
		t.Errorf("expected isMax=true after setting -1 (max), got false")
	}
}
