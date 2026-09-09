//go:build linux

package rootfs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

type pivotRootFunc func(newRoot string, debug bool) error
type deviceSetupFunc func(newRoot string, debug bool) error

type pivotRootOps struct {
	mount   func(source, target, fstype string, flags uintptr, data string) error
	mkdir   func(path string, mode os.FileMode) error
	pivot   func(newRoot, putOld string) error
	chdir   func(path string) error
	unmount func(target string, flags int) error
	remove  func(path string) error
}

func defaultPivotRootOps() pivotRootOps {
	return pivotRootOps{
		mount:   syscall.Mount,
		mkdir:   os.Mkdir,
		pivot:   syscall.PivotRoot,
		chdir:   syscall.Chdir,
		unmount: syscall.Unmount,
		remove:  os.Remove,
	}
}

func Isolate(newRoot string, debug bool) error {
	return isolateWithDeviceSetup(newRoot, debug, preparePrivateDevices, pivotRoot)
}

func isolateWithDeviceSetup(newRoot string, debug bool, setup deviceSetupFunc, pivot pivotRootFunc) error {
	if setup == nil {
		return fmt.Errorf("private /dev isolation function is nil")
	}
	if err := setup(newRoot, debug); err != nil {
		return fmt.Errorf("private /dev isolation required: %w", err)
	}
	return isolateWithPivot(newRoot, debug, pivot)
}

func isolateWithPivot(newRoot string, debug bool, pivot pivotRootFunc) error {
	if pivot == nil {
		return fmt.Errorf("pivot_root isolation function is nil")
	}
	if err := pivot(newRoot, debug); err != nil {
		return fmt.Errorf("pivot_root isolation required: %w", err)
	}
	return nil
}

func pivotRoot(newRoot string, debug bool) error {
	return pivotRootWithOps(newRoot, debug, defaultPivotRootOps())
}

func pivotRootWithOps(newRoot string, debug bool, ops pivotRootOps) (resultErr error) {
	if ops.mount == nil || ops.mkdir == nil || ops.pivot == nil || ops.chdir == nil || ops.unmount == nil || ops.remove == nil {
		return fmt.Errorf("pivot_root operations are incomplete")
	}

	// A new mount namespace inherits the parent's propagation topology. Make the
	// entire namespace recursively private before creating runtime mounts so a
	// shared/slave parent cannot receive rootfs mount events. This is a required
	// isolation boundary: failure must abort before touching newRoot.
	if err := ops.mount("", "/", "", syscall.MS_REC|syscall.MS_PRIVATE, ""); err != nil {
		return fmt.Errorf("make mount namespace recursively private: %w", err)
	}

	// pivot_root(2) requires newRoot to be a mount point. The recursive bind
	// preserves submounts already prepared inside the rootfs.
	if err := ops.mount(newRoot, newRoot, "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("bind-mount rootfs onto itself: %w", err)
	}
	rootBindMounted := true
	pivoted := false
	pivotDirOwned := false
	oldRootDetached := false
	pivotDir := filepath.Join(newRoot, ".pivot_old")

	defer func() {
		if resultErr == nil {
			return
		}
		if pivoted {
			cleanupPath := "/.pivot_old"
			if !oldRootDetached {
				if err := ops.unmount(cleanupPath, syscall.MNT_DETACH); err != nil {
					resultErr = errors.Join(resultErr, fmt.Errorf("rollback detach old root: %w", err))
				} else {
					oldRootDetached = true
				}
			}
			if pivotDirOwned && oldRootDetached {
				if err := ops.remove(cleanupPath); err != nil {
					resultErr = errors.Join(resultErr, fmt.Errorf("rollback remove %s: %w", cleanupPath, err))
				} else {
					pivotDirOwned = false
				}
			}
			return
		}
		if pivotDirOwned {
			if err := ops.remove(pivotDir); err != nil {
				resultErr = errors.Join(resultErr, fmt.Errorf("rollback remove %s: %w", pivotDir, err))
			} else {
				pivotDirOwned = false
			}
		}
		if rootBindMounted {
			if err := ops.unmount(newRoot, syscall.MNT_DETACH); err != nil {
				resultErr = errors.Join(resultErr, fmt.Errorf("rollback rootfs bind mount %s: %w", newRoot, err))
			} else {
				rootBindMounted = false
			}
		}
	}()

	if err := ops.mkdir(pivotDir, 0o700); err != nil {
		return fmt.Errorf("mkdir .pivot_old: %w", err)
	}
	pivotDirOwned = true

	if err := ops.pivot(newRoot, pivotDir); err != nil {
		return fmt.Errorf("pivot_root(%s, %s): %w", newRoot, pivotDir, err)
	}
	pivoted = true
	rootBindMounted = false

	if err := ops.chdir("/"); err != nil {
		return fmt.Errorf("chdir /: %w", err)
	}
	if err := ops.unmount("/.pivot_old", syscall.MNT_DETACH); err != nil {
		return fmt.Errorf("unmount old root: %w", err)
	}
	oldRootDetached = true
	if err := ops.remove("/.pivot_old"); err != nil {
		return fmt.Errorf("remove /.pivot_old: %w", err)
	}
	pivotDirOwned = false

	if debug {
		fmt.Println("[init] pivot_root complete")
	}
	return nil
}
