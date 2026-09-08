package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLoadOCIBundleTranslatesAdditionalGids(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("OCI process user execution is Linux-specific")
	}
	bundle := t.TempDir()
	if err := os.Mkdir(filepath.Join(bundle, "rootfs"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := `{
  "ociVersion":"1.1.0",
  "root":{"path":"rootfs"},
  "process":{"args":["/bin/true"],"cwd":"/","user":{"uid":123,"gid":456,"additionalGids":[789,790]}}
}`
	if err := os.WriteFile(filepath.Join(bundle, "config.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadOCIBundle(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ProcessUser == nil || cfg.ProcessUser.UID != 123 || cfg.ProcessUser.GID != 456 {
		t.Fatalf("process user = %#v, want 123:456", cfg.ProcessUser)
	}
	if len(cfg.ProcessUser.Groups) != 2 || cfg.ProcessUser.Groups[0] != 789 || cfg.ProcessUser.Groups[1] != 790 {
		t.Fatalf("supplementary groups = %v, want [789 790]", cfg.ProcessUser.Groups)
	}
}

func TestLoadOCIBundleRejectsAdditionalGidsWithUserNamespace(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("OCI process user execution is Linux-specific")
	}
	bundle := t.TempDir()
	if err := os.Mkdir(filepath.Join(bundle, "rootfs"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := `{
  "ociVersion":"1.1.0",
  "root":{"path":"rootfs"},
  "process":{"args":["/bin/true"],"cwd":"/","user":{"uid":0,"gid":0,"additionalGids":[0]}},
  "linux":{"namespaces":[{"type":"user"}]}
}`
	if err := os.WriteFile(filepath.Join(bundle, "config.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := loadOCIBundle(bundle)
	if err == nil || !strings.Contains(err.Error(), "setgroups") {
		t.Fatalf("error = %v, want user namespace setgroups fail-closed rejection", err)
	}
}

func TestTranslateOCIProcessUserRejectsDuplicateAdditionalGids(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("OCI process user execution is Linux-specific")
	}
	_, err := translateOCIProcessUser(&ociProcessUserConfig{UID: 1, GID: 2, AdditionalGids: []uint32{3, 3}})
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("error = %v, want duplicate supplementary gid rejection", err)
	}
}
