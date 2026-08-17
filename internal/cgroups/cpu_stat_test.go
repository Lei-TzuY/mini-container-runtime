package cgroups

import "testing"

func TestReadCPUStat(t *testing.T) {
	tmpDir := t.TempDir()
	writePath := tmpDir + "/cpu.stat"
	if err := os.WriteFile(writePath, []byte("usage_usec 100\nuser_usec 60\nsystem_usec 40\n"), 0o644); err != nil {
		t.Fatalf("write cpu.stat fixture: %v", err)
	}
	metrics, err := ReadCPUStat(tmpDir)
	if err != nil {
		t.Fatalf("ReadCPUStat error: %v", err)
	}
	if metrics["usage_usec"] != 100 {
		t.Fatalf("usage_usec = %d, want 100", metrics["usage_usec"])
	}
}
