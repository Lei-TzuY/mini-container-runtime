//go:build linux

package container

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestMountMaskedDirectoryHidesContentsAndIsReadOnly(t *testing.T) {
	target := t.TempDir()
	secret := filepath.Join(target, "secret")
	if err := os.WriteFile(secret, []byte("sensitive"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := mountMaskedDirectory(target)
	if err != nil {
		if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
			t.Skipf("mount namespace lacks CAP_SYS_ADMIN: %v", err)
		}
		t.Fatalf("mountMaskedDirectory: %v", err)
	}
	t.Cleanup(func() { _ = syscall.Unmount(target, syscall.MNT_DETACH) })

	if _, err := os.Stat(secret); !os.IsNotExist(err) {
		t.Fatalf("masked directory still exposes pre-mount content: stat err=%v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "new"), []byte("x"), 0o600); err == nil {
		t.Fatal("masked directory accepted a write; want read-only tmpfs")
	}
}

func TestMountMaskedDirectoryRejectsNonCanonicalTarget(t *testing.T) {
	for _, target := range []string{"", "relative/path", "/tmp/../tmp"} {
		if err := mountMaskedDirectory(target); err == nil {
			t.Fatalf("mountMaskedDirectory(%q) succeeded, want validation error", target)
		}
	}
}
