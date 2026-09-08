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

func TestOCIRlimitNOFILEBecomesDurableRuntimeMarker(t *testing.T) {
	bundle := writeOCIRlimitBundle(t, `{"args":["/bin/true"],"cwd":"/","env":["A=B"],"rlimits":[{"type":"RLIMIT_NOFILE","soft":64,"hard":128}]}`)
	cfg, err := loadOCIBundle(bundle)
	if err != nil {
		t.Fatalf("load OCI bundle: %v", err)
	}
	want := processRlimitNOFILEEnv + "=64:128"
	if len(cfg.Env) != 2 || cfg.Env[0] != "A=B" || cfg.Env[1] != want {
		t.Fatalf("runtime env = %#v, want [A=B %s]", cfg.Env, want)
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
	bundle := writeOCIRlimitBundle(t, `{"args":["/bin/true"],"cwd":"/","rlimits":[{"type":"RLIMIT_NOFILE","soft":129,"hard":128}]}`)
	_, err := loadOCIBundle(bundle)
	if err == nil || !strings.Contains(err.Error(), "exceeds hard limit") {
		t.Fatalf("load error = %v, want soft/hard validation", err)
	}
}

func TestOCIRlimitRejectsDuplicateResource(t *testing.T) {
	bundle := writeOCIRlimitBundle(t, `{"args":["/bin/true"],"cwd":"/","rlimits":[{"type":"RLIMIT_NOFILE","soft":64,"hard":128},{"type":"RLIMIT_NOFILE","soft":32,"hard":64}]}`)
	_, err := loadOCIBundle(bundle)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("load error = %v, want duplicate validation", err)
	}
}

func TestOCIRlimitRejectsRuntimeMarkerCollision(t *testing.T) {
	process := fmt.Sprintf(`{"args":["/bin/true"],"cwd":"/","env":[%q],"rlimits":[{"type":"RLIMIT_NOFILE","soft":64,"hard":128}]}`, processRlimitNOFILEEnv+"=attacker")
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
