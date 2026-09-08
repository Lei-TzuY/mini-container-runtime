//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadOCIBundleAdmitsProcessOOMScoreAdj(t *testing.T) {
	bundle := t.TempDir()
	if err := os.Mkdir(filepath.Join(bundle, "rootfs"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := `{
  "ociVersion":"1.1.0",
  "root":{"path":"rootfs"},
  "process":{"args":["/bin/true"],"cwd":"/","oomScoreAdj":500}
}`
	if err := os.WriteFile(filepath.Join(bundle, "config.json"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadOCIBundle(bundle)
	if err != nil {
		t.Fatalf("load OCI bundle: %v", err)
	}
	want := processOOMScoreAdjEnv + "=500"
	found := false
	for _, entry := range cfg.Env {
		if entry == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("runtime env = %q, want %q", cfg.Env, want)
	}
}

func TestLoadOCIBundleRejectsInvalidProcessOOMScoreAdj(t *testing.T) {
	for _, value := range []string{"-1001", "1001", "null"} {
		t.Run(value, func(t *testing.T) {
			bundle := t.TempDir()
			if err := os.Mkdir(filepath.Join(bundle, "rootfs"), 0o755); err != nil {
				t.Fatal(err)
			}
			config := `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/","oomScoreAdj":` + value + `}}`
			if err := os.WriteFile(filepath.Join(bundle, "config.json"), []byte(config), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := loadOCIBundle(bundle)
			if err == nil || !strings.Contains(err.Error(), "oomScoreAdj") {
				t.Fatalf("load error = %v, want oomScoreAdj rejection", err)
			}
		})
	}
}

func TestLoadOCIBundleRejectsProcessOOMScoreMarkerCollision(t *testing.T) {
	bundle := t.TempDir()
	if err := os.Mkdir(filepath.Join(bundle, "rootfs"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := `{
  "ociVersion":"1.1.0",
  "root":{"path":"rootfs"},
  "process":{"args":["/bin/true"],"cwd":"/","oomScoreAdj":500,"env":["MINICONTAINER_PROCESS_OOM_SCORE_ADJ=1"]}
}`
	if err := os.WriteFile(filepath.Join(bundle, "config.json"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := loadOCIBundle(bundle)
	if err == nil || !strings.Contains(err.Error(), "conflicts with internal oom score policy") {
		t.Fatalf("load error = %v, want marker collision rejection", err)
	}
}
