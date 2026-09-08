//go:build linux

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeOCIRlimitBundle(t *testing.T, process string) string {
	t.Helper()
	bundle := t.TempDir()
	if err := os.Mkdir(filepath.Join(bundle, "rootfs"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := fmt.Sprintf(`{
  "ociVersion":"1.1.0",
  "root":{"path":"rootfs"},
  "process":%s
}`, process)
	if err := os.WriteFile(filepath.Join(bundle, "config.json"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	return bundle
}

func TestOCIRlimitsBecomeDurableRuntimeMarkers(t *testing.T) {
	bundle := writeOCIRlimitBundle(t, `{"args":["/bin/true"],"cwd":"/","env":["A=B"],"rlimits":[{"type":"RLIMIT_NOFILE","soft":64,"hard":128},{"type":"RLIMIT_CORE","soft":0,"hard":0},{"type":"RLIMIT_FSIZE","soft":4096,"hard":8192}]}`)
	cfg, err := loadOCIBundle(bundle)
	if err != nil {
		t.Fatalf("load OCI bundle: %v", err)
	}
	want := []string{
		"A=B",
		processRlimitNOFILEEnv + "=64:128",
		processRlimitCOREEnv + "=0:0",
		processRlimitFSIZEEnv + "=4096:8192",
	}
	if len(cfg.Env) != len(want) {
		t.Fatalf("runtime env = %#v, want %#v", cfg.Env, want)
	}
	for i := range want {
		if cfg.Env[i] != want[i] {
			t.Fatalf("runtime env[%d] = %q, want %q", i, cfg.Env[i], want[i])
		}
	}
}

func TestOCIRlimitRejectsUnsupportedResource(t *testing.T) {
	bundle := writeOCIRlimitBundle(t, `{"args":["/bin/true"],"cwd":"/","rlimits":[{"type":"RLIMIT_NPROC","soft":1,"hard":1}]}`)
	_, err := loadOCIBundle(bundle)
	if err == nil || !strings.Contains(err.Error(), "RLIMIT_NPROC") {
		t.Fatalf("load error = %v, want unsupported RLIMIT_NPROC", err)
	}
}

func TestOCIRlimitRejectsSoftAboveHard(t *testing.T) {
	bundle := writeOCIRlimitBundle(t, `{"args":["/bin/true"],"cwd":"/","rlimits":[{"type":"RLIMIT_FSIZE","soft":129,"hard":128}]}`)
	_, err := loadOCIBundle(bundle)
	if err == nil || !strings.Contains(err.Error(), "exceeds hard limit") {
		t.Fatalf("load error = %v, want soft/hard validation", err)
	}
}

func TestOCIRlimitRejectsDuplicateResource(t *testing.T) {
	bundle := writeOCIRlimitBundle(t, `{"args":["/bin/true"],"cwd":"/","rlimits":[{"type":"RLIMIT_CORE","soft":0,"hard":0},{"type":"RLIMIT_CORE","soft":0,"hard":0}]}`)
	_, err := loadOCIBundle(bundle)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("load error = %v, want duplicate validation", err)
	}
}

func TestOCIRlimitRejectsRuntimeMarkerCollision(t *testing.T) {
	process := fmt.Sprintf(`{"args":["/bin/true"],"cwd":"/","env":[%q],"rlimits":[{"type":"RLIMIT_CORE","soft":0,"hard":0}]}`, processRlimitCOREEnv+"=attacker")
	bundle := writeOCIRlimitBundle(t, process)
	_, err := loadOCIBundle(bundle)
	if err == nil || !strings.Contains(err.Error(), "conflicts with internal rlimit policy") {
		t.Fatalf("load error = %v, want reserved marker collision", err)
	}
}

func TestOCIRlimitKeepsStrictUnknownFieldValidation(t *testing.T) {
	bundle := writeOCIRlimitBundle(t, `{"args":["/bin/true"],"cwd":"/","rlimits":[{"type":"RLIMIT_NOFILE","soft":64,"hard":128,"bogus":true}]}`)
	_, err := loadOCIBundle(bundle)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("load error = %v, want strict unknown-field rejection", err)
	}
}
