//go:build linux

package container

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

func TestOpenVolumeTargetPinsExistingRegularFile(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "etc", "secret")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	fd, err := openVolumeTarget(root, "/etc/secret")
	if err != nil {
		t.Fatalf("open file target: %v", err)
	}
	defer unix.Close(fd)
	want, err := os.Stat(target)
	if err != nil { t.Fatal(err) }
	got, err := os.Stat(volumeTargetFDPath(fd))
	if err != nil { t.Fatal(err) }
	if got.IsDir() || !os.SameFile(want, got) {
		t.Fatal("target fd does not pin the existing regular file")
	}
}

func TestExistingFileTargetCannotEscapeRootThroughSymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret")
	if err := os.WriteFile(outsideFile, []byte("outside"), 0o600); err != nil { t.Fatal(err) }
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil { t.Fatal(err) }
	fd, err := openVolumeTarget(root, "/escape/secret")
	if fd >= 0 { _ = unix.Close(fd) }
	if err == nil {
		t.Fatal("file target escaped rootfs through absolute symlink")
	}
}

func TestMountVolumeBindsDevNullOverExistingFile(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "secret")
	if err := os.WriteFile(target, []byte("sensitive"), 0o600); err != nil { t.Fatal(err) }
	err := mountVolume(Volume{HostPath: "/dev/null", ContainerPath: "/secret", ReadOnly: true}, root, false)
	if err != nil {
		if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
			t.Skipf("mount namespace lacks CAP_SYS_ADMIN: %v", err)
		}
		t.Fatalf("mount masked file: %v", err)
	}
	defer syscall.Unmount(target, syscall.MNT_DETACH)
	data, err := os.ReadFile(target)
	if err != nil { t.Fatalf("read masked file: %v", err) }
	if len(data) != 0 {
		t.Fatalf("masked file exposed %q", data)
	}
	if err := os.WriteFile(target, []byte("write"), 0o600); err == nil {
		t.Fatal("read-only masked file accepted a write")
	}
}
