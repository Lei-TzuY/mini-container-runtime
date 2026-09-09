//go:build linux

package container

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestMountMaskedDirectoryInRootHidesDirectoryAndFailsClosed(t *testing.T) {
	rootfs := t.TempDir()
	target := filepath.Join(rootfs, "secrets")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "token"), []byte("sensitive"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := mountMaskedDirectoryInRoot(rootfs, "/secrets")
	if err != nil {
		if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
			// Restricted runners lack CAP_SYS_ADMIN. This is the required fail-closed
			// kernel boundary: the runtime returns the mount error before payload exec.
			return
		}
		t.Fatalf("mountMaskedDirectoryInRoot: %v", err)
	}
	defer syscall.Unmount(target, syscall.MNT_DETACH)

	entries, err := os.ReadDir(target)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("masked directory leaked entries: %#v", entries)
	}
	if err := os.WriteFile(filepath.Join(target, "new"), []byte("x"), 0o600); err == nil {
		t.Fatal("masked directory accepted a write")
	}
}

func TestMaskedDirectoryTransportScrubsInheritedPolicy(t *testing.T) {
	env, err := appendMaskedDirectoriesEnv([]string{
		"PATH=/bin",
		maskedDirectoriesEnvKey + `=["/attacker"]`,
	}, []string{"/secrets"})
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, entry := range env {
		if len(entry) >= len(maskedDirectoriesEnvKey)+1 && entry[:len(maskedDirectoriesEnvKey)+1] == maskedDirectoriesEnvKey+"=" {
			found++
			if entry != maskedDirectoriesEnvKey+`=["/secrets"]` {
				t.Fatalf("masked policy transport = %q", entry)
			}
		}
	}
	if found != 1 {
		t.Fatalf("masked policy env entries = %d, want 1", found)
	}
}
