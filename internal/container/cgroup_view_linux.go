//go:build linux

package container

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

const hostCgroupV2Mount = "/sys/fs/cgroup"

type cgroupViewMountOps struct {
	mkdirAll func(string, os.FileMode) error
	mount    func(string, string, string, uintptr, string) error
}

func mountCgroupV2View(rootfs string, enabled, debug bool) error {
	return mountCgroupV2ViewWithOps(rootfs, enabled, debug, cgroupViewMountOps{
		mkdirAll: os.MkdirAll,
		mount:    syscall.Mount,
	})
}

// mountCgroupV2ViewWithOps exposes the host cgroup-v2 filesystem read-only in
// the container mount namespace. The payload needs this view to observe the
// limits already applied by the parent, but must not be able to rewrite host
// cgroup controls.
func mountCgroupV2ViewWithOps(rootfs string, enabled, debug bool, ops cgroupViewMountOps) error {
	if !enabled {
		return nil
	}
	if rootfs == "" {
		return fmt.Errorf("cgroup view rootfs is empty")
	}
	if ops.mkdirAll == nil || ops.mount == nil {
		return fmt.Errorf("cgroup view mount operations are incomplete")
	}

	target := filepath.Join(rootfs, "sys", "fs", "cgroup")
	if err := ops.mkdirAll(target, 0o755); err != nil {
		return fmt.Errorf("create cgroup v2 mountpoint: %w", err)
	}
	if err := ops.mount(hostCgroupV2Mount, target, "", syscall.MS_BIND, ""); err != nil {
		return fmt.Errorf("bind cgroup v2 view: %w", err)
	}
	flags := uintptr(syscall.MS_BIND | syscall.MS_REMOUNT | syscall.MS_RDONLY | syscall.MS_NOSUID | syscall.MS_NODEV | syscall.MS_NOEXEC)
	if err := ops.mount("", target, "", flags, ""); err != nil {
		return fmt.Errorf("remount cgroup v2 view read-only: %w", err)
	}
	if debug {
		fmt.Printf("[init] cgroup v2 view mounted read-only at %q\n", target)
	}
	return nil
}
