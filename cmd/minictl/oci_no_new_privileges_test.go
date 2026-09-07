package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOCIBundleTranslatesNoNewPrivileges(t *testing.T) {
	bundle := t.TempDir()
	if err := os.Mkdir(filepath.Join(bundle, "rootfs"), 0755); err != nil {
		t.Fatal(err)
	}
	config := `{
  "ociVersion": "1.1.0",
  "root": {"path": "rootfs"},
  "process": {
    "args": ["/bin/true"],
    "cwd": "/",
    "noNewPrivileges": true
  },
  "linux": {"namespaces": []}
}`
	if err := os.WriteFile(filepath.Join(bundle, "config.json"), []byte(config), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadOCIBundle(bundle)
	if err != nil {
		t.Fatalf("loadOCIBundle() error = %v", err)
	}
	if !cfg.NoNewPrivileges {
		t.Fatal("NoNewPrivileges = false, want true")
	}
}
