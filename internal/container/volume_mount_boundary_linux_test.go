//go:build linux

package container

import (
	"errors"
	"testing"

	"golang.org/x/sys/unix"
)

func TestOpenat2VolumeTargetRejectsMountBoundary(t *testing.T) {
	rootFD, err := unix.Open("/", unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(rootFD)
	procFD, err := unix.Open("/proc", unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Skipf("/proc unavailable: %v", err)
	}
	rootMount, err := fdMountID(rootFD)
	if err != nil {
		t.Fatal(err)
	}
	procMount, err := fdMountID(procFD)
	_ = unix.Close(procFD)
	if err != nil {
		t.Fatal(err)
	}
	if rootMount == procMount {
		t.Skip("/proc is not a separate mount in this environment")
	}

	fd, err := openVolumeDirInRoot(rootFD, "proc")
	if fd >= 0 {
		_ = unix.Close(fd)
	}
	if errors.Is(err, unix.ENOSYS) {
		t.Skip("kernel does not support openat2")
	}
	if !errors.Is(err, unix.EXDEV) {
		t.Fatalf("cross-mount openat2 error=%v, want EXDEV", err)
	}
}

func TestNoSymlinkFallbackRejectsMountBoundary(t *testing.T) {
	rootFD, err := unix.Open("/", unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(rootFD)
	procFD, err := unix.Open("/proc", unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Skipf("/proc unavailable: %v", err)
	}
	rootMount, err := fdMountID(rootFD)
	if err != nil {
		t.Fatal(err)
	}
	procMount, err := fdMountID(procFD)
	_ = unix.Close(procFD)
	if err != nil {
		t.Fatal(err)
	}
	if rootMount == procMount {
		t.Skip("/proc is not a separate mount in this environment")
	}

	fd, err := openOrCreateVolumeTargetNoSymlink(rootFD, "proc")
	if fd >= 0 {
		_ = unix.Close(fd)
	}
	if err == nil {
		t.Fatal("fallback crossed into /proc mount")
	}
}

func TestFDMountIDIsStableWithinSameMount(t *testing.T) {
	root := t.TempDir()
	rootFD, err := unix.Open(root, unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(rootFD)
	if err := unix.Mkdirat(rootFD, "child", 0o755); err != nil {
		t.Fatal(err)
	}
	childFD, err := unix.Openat(rootFD, "child", unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(childFD)

	rootMount, err := fdMountID(rootFD)
	if err != nil {
		t.Fatal(err)
	}
	childMount, err := fdMountID(childFD)
	if err != nil {
		t.Fatal(err)
	}
	if rootMount != childMount {
		t.Fatalf("same-mount directory IDs differ: root=%d child=%d", rootMount, childMount)
	}
}
