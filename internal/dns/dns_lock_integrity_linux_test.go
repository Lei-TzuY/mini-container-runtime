//go:build linux

package dns

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDNSNetworkLockRejectsMultipleHardLinks(t *testing.T) {
	dir := t.TempDir()
	const networkName = "default"
	lockPath := filepath.Join(dir, networkName+".lock")
	if err := os.WriteFile(lockPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(dir, "alias.lock")
	if err := os.Link(lockPath, alias); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}

	called := false
	err := withDNSNetworkLock(dir, networkName, func() error {
		called = true
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "exactly one link") {
		t.Fatalf("hard-linked lock error=%v, want single-link rejection", err)
	}
	if called {
		t.Fatal("callback ran with ambiguous hard-linked lock authority")
	}
}

func TestDNSNetworkLockDetectsUnlinkReplacementDuringCriticalSection(t *testing.T) {
	dir := t.TempDir()
	const networkName = "default"
	lockPath := filepath.Join(dir, networkName+".lock")

	err := withDNSNetworkLock(dir, networkName, func() error {
		if err := os.Remove(lockPath); err != nil {
			return err
		}
		if err := os.WriteFile(lockPath, []byte("replacement"), 0o600); err != nil {
			return err
		}
		return nil
	})
	if err == nil || (!strings.Contains(err.Error(), "path changed while locked") && !strings.Contains(err.Error(), "exactly one link, got 0")) {
		t.Fatalf("replaced lock error=%v, want inode integrity rejection", err)
	}
	data, readErr := os.ReadFile(lockPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != "replacement" {
		t.Fatalf("replacement lock data=%q", data)
	}
}

func TestDNSNetworkLockRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	const networkName = "default"
	outside := filepath.Join(t.TempDir(), "outside.lock")
	if err := os.WriteFile(outside, []byte("sentinel"), 0o600); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(dir, networkName+".lock")
	if err := os.Symlink(outside, lockPath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	called := false
	if err := withDNSNetworkLock(dir, networkName, func() error {
		called = true
		return nil
	}); err == nil {
		t.Fatal("symlinked lock was accepted")
	}
	if called {
		t.Fatal("callback ran with symlinked lock")
	}
	data, err := os.ReadFile(outside)
	if err != nil || string(data) != "sentinel" {
		t.Fatalf("outside lock target changed: data=%q err=%v", data, err)
	}
}

func TestDNSNetworkLockRejectsSymlinkedRegistryDirectory(t *testing.T) {
	realDir := t.TempDir()
	parent := t.TempDir()
	dir := filepath.Join(parent, "dns")
	if err := os.Symlink(realDir, dir); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	called := false
	err := withDNSNetworkLock(dir, "default", func() error {
		called = true
		return nil
	})
	if err == nil {
		t.Fatal("symlinked DNS registry directory was accepted")
	}
	if called {
		t.Fatal("callback ran with symlinked DNS registry directory")
	}
}

func TestDNSNetworkLockDetectsDirectoryReplacementDuringCriticalSection(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "dns")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(parent, "dns-old")

	err := withDNSNetworkLock(dir, "default", func() error {
		if err := os.Rename(dir, moved); err != nil {
			return err
		}
		if err := os.Mkdir(dir, 0o700); err != nil {
			return err
		}
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "directory for \"default\" changed while locked") {
		t.Fatalf("replaced directory error=%v, want directory identity rejection", err)
	}
}
