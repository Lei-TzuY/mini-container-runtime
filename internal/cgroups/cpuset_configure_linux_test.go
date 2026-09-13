//go:build linux

package cgroups

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigureV2AppliesCPUSetBeforeAdmission(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{CPUSetCPUs: "0-2", CPUSetMems: "0"}
	if err := configureV2(dir, 4242, cfg, false); err != nil {
		t.Fatalf("configure v2 cpuset: %v", err)
	}
	for name, want := range map[string]string{
		"cpuset.cpus": "0-2",
		"cpuset.mems": "0",
		"cgroup.procs": "4242",
	} {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if string(got) != want {
			t.Fatalf("%s = %q, want %q", name, got, want)
		}
	}
}
