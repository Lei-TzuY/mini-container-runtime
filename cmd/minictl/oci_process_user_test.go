package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLoadOCIBundleTranslatesProcessUser(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("OCI process user is Linux-specific")
	}
	bundle := t.TempDir()
	if err := os.Mkdir(filepath.Join(bundle, "rootfs"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := `{
  "ociVersion":"1.1.0",
  "root":{"path":"rootfs"},
  "process":{"args":["/bin/true"],"cwd":"/","user":{"uid":0,"gid":0}},
  "linux":{"namespaces":[{"type":"user"}]}
}`
	if err := os.WriteFile(filepath.Join(bundle, "config.json"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadOCIBundle(bundle)
	if err != nil {
		t.Fatalf("load OCI bundle: %v", err)
	}
	if cfg.ProcessUser == nil || cfg.ProcessUser.UID != 0 || cfg.ProcessUser.GID != 0 {
		t.Fatalf("process user = %#v, want 0:0", cfg.ProcessUser)
	}
}

func TestLoadOCIBundleRejectsUnmappedProcessUser(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("OCI process user is Linux-specific")
	}
	bundle := t.TempDir()
	if err := os.Mkdir(filepath.Join(bundle, "rootfs"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := `{
  "ociVersion":"1.1.0",
  "root":{"path":"rootfs"},
  "process":{"args":["/bin/true"],"cwd":"/","user":{"uid":1000,"gid":1000}},
  "linux":{"namespaces":[{"type":"user"}]}
}`
	if err := os.WriteFile(filepath.Join(bundle, "config.json"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := loadOCIBundle(bundle)
	if err == nil || !strings.Contains(err.Error(), "one-entry user namespace mapping") {
		t.Fatalf("load error = %v, want unmapped process user rejection", err)
	}
}

func TestTranslateOCIProcessUserSupportsAdditionalGids(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("OCI process user is Linux-specific")
	}
	user, err := translateOCIProcessUser(&ociProcessUserConfig{UID: 1, GID: 2, AdditionalGids: []uint32{3, 4}})
	if err != nil {
		t.Fatalf("translate process user: %v", err)
	}
	if user == nil || user.UID != 1 || user.GID != 2 || len(user.Groups) != 2 || user.Groups[0] != 3 || user.Groups[1] != 4 {
		t.Fatalf("translated process user = %#v, want 1:2 groups [3 4]", user)
	}
}

func TestTranslateOCIProcessUserSupportsUmask(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("OCI process user is Linux-specific")
	}
	umask := uint32(0o027)
	user, err := translateOCIProcessUser(&ociProcessUserConfig{UID: 1, GID: 2, Umask: &umask})
	if err != nil {
		t.Fatalf("translate process user: %v", err)
	}
	if user == nil || user.Umask == nil || *user.Umask != umask {
		t.Fatalf("translated process user = %#v, want umask %#o", user, umask)
	}
}

func TestTranslateOCIProcessUserRejectsInvalidUmask(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("OCI process user is Linux-specific")
	}
	umask := uint32(0o1000)
	_, err := translateOCIProcessUser(&ociProcessUserConfig{UID: 1, GID: 2, Umask: &umask})
	if err == nil || !strings.Contains(err.Error(), "umask") {
		t.Fatalf("translate error = %v, want invalid umask rejection", err)
	}
}
