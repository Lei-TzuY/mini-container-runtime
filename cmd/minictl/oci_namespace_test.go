package main

import (
	"strings"
	"testing"
)

func TestLoadOCIBundleUserNamespaceTracksOCIConfig(t *testing.T) {
	withUser := writeOCIBundle(t, `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"linux":{"namespaces":[{"type":"pid"},{"type":"user"}]}}`)
	cfg, err := loadOCIBundle(withUser)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.UserNS {
		t.Fatal("user namespace requested by OCI config was not enabled")
	}

	withoutUser := writeOCIBundle(t, `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"linux":{"namespaces":[{"type":"pid"},{"type":"mount"}]}}`)
	cfg, err = loadOCIBundle(withoutUser)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UserNS {
		t.Fatal("user namespace omitted by OCI config was silently enabled")
	}
}

func TestLoadOCIBundleRejectsUnimplementedCgroupNamespace(t *testing.T) {
	bundle := writeOCIBundle(t, `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"linux":{"namespaces":[{"type":"cgroup"}]}}`)
	_, err := loadOCIBundle(bundle)
	if err == nil || !strings.Contains(err.Error(), "cgroup namespace is not supported") {
		t.Fatalf("error=%v", err)
	}
}

func TestLoadOCIBundleRejectsDuplicateNamespaces(t *testing.T) {
	bundle := writeOCIBundle(t, `{"ociVersion":"1.1.0","root":{"path":"rootfs"},"process":{"args":["/bin/true"],"cwd":"/"},"linux":{"namespaces":[{"type":"user"},{"type":"user"}]}}`)
	_, err := loadOCIBundle(bundle)
	if err == nil || !strings.Contains(err.Error(), "duplicate linux namespace") {
		t.Fatalf("error=%v", err)
	}
}
