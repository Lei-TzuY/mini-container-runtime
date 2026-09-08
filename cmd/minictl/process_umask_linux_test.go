//go:build linux

package main

import (
	"os"
	"path/filepath"
	"testing"
)

const processUmaskProbeFileEnv = "MINICONTAINER_TEST_PROCESS_UMASK_PROBE_FILE"

func TestProcessUmaskProbe(t *testing.T) {
	path := os.Getenv(processUmaskProbeFileEnv)
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o666)
	if err != nil {
		t.Fatalf("create probe file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close probe file: %v", err)
	}
}

func TestRunContainerInitSupervisorAppliesProcessUmask(t *testing.T) {
	probe := filepath.Join(t.TempDir(), "probe")
	t.Setenv(processUmaskRuntimeEnv, "63") // 0077
	t.Setenv(processUmaskProbeFileEnv, probe)

	code, err := runContainerInitSupervisor([]string{os.Args[0], "-test.run=^TestProcessUmaskProbe$"})
	if err != nil {
		t.Fatalf("run supervisor: %v", err)
	}
	if code != 0 {
		t.Fatalf("payload exit code = %d, want 0", code)
	}
	info, err := os.Stat(probe)
	if err != nil {
		t.Fatalf("stat probe file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("probe mode = %#o, want 0600 for umask 0077", got)
	}
}

func TestPayloadUmaskRuntimeMarkerRejectsOutOfRangeValue(t *testing.T) {
	t.Setenv(processUmaskRuntimeEnv, "512") // 01000
	_, err := payloadUmaskFromRuntimeEnv()
	if err == nil {
		t.Fatal("expected invalid process umask marker error")
	}
}
