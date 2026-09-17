//go:build linux

package container

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConsumePinnedRootFSPathUsesFallbackWithoutMarker(t *testing.T) {
	_ = os.Unsetenv(pinnedRootFSPathEnvKey)
	got, err := consumePinnedRootFSPath("/admitted/rootfs")
	if err != nil {
		t.Fatalf("consumePinnedRootFSPath: %v", err)
	}
	if got != "/admitted/rootfs" {
		t.Fatalf("path = %q, want fallback", got)
	}
}

func TestConsumePinnedRootFSPathConsumesMarker(t *testing.T) {
	t.Setenv(pinnedRootFSPathEnvKey, "/original/rootfs")
	got, err := consumePinnedRootFSPath("/proc/self/fd/5")
	if err != nil {
		t.Fatalf("consumePinnedRootFSPath: %v", err)
	}
	if got != "/original/rootfs" {
		t.Fatalf("path = %q, want original admission path", got)
	}
	if _, ok := os.LookupEnv(pinnedRootFSPathEnvKey); ok {
		t.Fatal("pinned rootfs path marker survived consumption")
	}
}

func TestConsumePinnedRootFSPathRejectsEmptyMarker(t *testing.T) {
	t.Setenv(pinnedRootFSPathEnvKey, "")
	if _, err := consumePinnedRootFSPath("/fallback"); err == nil {
		t.Fatal("empty pinned rootfs marker unexpectedly accepted")
	}
	if _, ok := os.LookupEnv(pinnedRootFSPathEnvKey); ok {
		t.Fatal("invalid pinned rootfs marker survived rejection")
	}
}

func TestAttachPinnedRootFSRejectsPathIdentityChangeBeforeMount(t *testing.T) {
	pinnedDir := t.TempDir()
	replacementDir := t.TempDir()
	pinned, err := os.Open(pinnedDir)
	if err != nil {
		t.Fatalf("open pinned rootfs: %v", err)
	}
	defer pinned.Close()

	target := filepath.Join(t.TempDir(), "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatalf("create target: %v", err)
	}
	pinnedPath := fmt.Sprintf("/proc/self/fd/%d", pinned.Fd())
	err = attachPinnedRootFS(pinnedPath, replacementDir, target)
	if err == nil || !strings.Contains(err.Error(), "identity changed") {
		t.Fatalf("identity replacement error = %v", err)
	}
}

func TestAttachPinnedRootFSRejectsEmptyInputs(t *testing.T) {
	for _, args := range [][3]string{
		{"", "/rootfs", "/target"},
		{"/proc/self/fd/5", "", "/target"},
		{"/proc/self/fd/5", "/rootfs", ""},
	} {
		if err := attachPinnedRootFS(args[0], args[1], args[2]); err == nil {
			t.Fatalf("empty input %#v unexpectedly accepted", args)
		}
	}
}
