//go:build linux

// internal/rootfs/rootfs_linux.go
//
// Filesystem Isolation — pivot_root
// ───────────────────────────────
// Container filesystem isolation means the container process sees a
// completely different directory tree than the host. This runtime requires
// pivot_root(2): unlike chroot(2), pivot_root lets us detach the old root mount
// entirely instead of silently retaining a weaker filesystem-isolation model.
//
// pivot_root algorithm
// ────────────────────
//  1. Bind-mount newRoot on itself → creates a mount-point entry in the
//     kernel's mount table, which pivot_root requires.
//  2. mkdir newRoot/.pivot_old  → parking spot for the old root.
//  3. pivot_root(newRoot, newRoot/.pivot_old)
//       → kernel swaps "/" to newRoot, old "/" is at /.pivot_old.
//  4. chdir "/"  → update CWD to the new root.
//  5. umount2("/.pivot_old", MNT_DETACH)
//       → lazy-unmount: detached immediately but stays accessible to
//         existing open file descriptors until they're all closed.
//  6. rmdir "/.pivot_old"  → clean up.

package rootfs

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

type pivotRootFunc func(newRoot string, debug bool) error

// Isolate changes the root filesystem of the current process to newRoot.
// It must be called after the process has entered a new mount namespace
// (CLONE_NEWNS), otherwise the bind-mount in step 1 would affect the host.
//
// Filesystem isolation is fail-closed: if pivot_root cannot be established we
// return the failure instead of downgrading to chroot, whose weaker mount-table
// isolation is not an equivalent container security boundary.
func Isolate(newRoot string, debug bool) error {
	return isolateWithPivot(newRoot, debug, pivotRoot)
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

// pivotRoot implements filesystem isolation using the pivot_root(2) syscall.
// This is the technique used by runc and Docker's default runtime.
func pivotRoot(newRoot string, debug bool) error {
	// Step 1: bind-mount newRoot onto itself.
	//
	// pivot_root(2) requires newRoot to already be a mount point. A plain
	// directory is NOT a mount point. Bind-mounting the directory onto itself
	// promotes it to a mount point without changing its contents.
	//
	// MS_REC propagates the bind to all sub-mounts (e.g., /proc and /sys
	// that ContainerInit mounted inside newRoot before calling us).
	if err := syscall.Mount(newRoot, newRoot, "",
		syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("bind-mount rootfs onto itself: %w", err)
	}

	// Step 2: create a temporary directory inside newRoot for the old root.
	pivotDir := filepath.Join(newRoot, ".pivot_old")
	if err := os.MkdirAll(pivotDir, 0700); err != nil {
		return fmt.Errorf("mkdir .pivot_old: %w", err)
	}

	// Step 3: invoke pivot_root.
	//   newRoot  → becomes the new "/"
	//   pivotDir → old "/" is bind-mounted here (visible as /.pivot_old)
	if err := syscall.PivotRoot(newRoot, pivotDir); err != nil {
		return fmt.Errorf("pivot_root(%s, %s): %w", newRoot, pivotDir, err)
	}

	// Step 4: update our CWD to the new root.
	// After pivot_root the process's CWD is still conceptually in the old
	// root (the kernel hasn't changed it automatically).
	if err := syscall.Chdir("/"); err != nil {
		return fmt.Errorf("chdir /: %w", err)
	}

	// Step 5: unmount the old root with MNT_DETACH (lazy unmount).
	// MNT_DETACH detaches the mount immediately from the filesystem tree
	// while keeping it accessible to any process that already has it open.
	if err := syscall.Unmount("/.pivot_old", syscall.MNT_DETACH); err != nil {
		return fmt.Errorf("unmount old root: %w", err)
	}

	// Step 6: remove the now-empty directory.
	if err := os.Remove("/.pivot_old"); err != nil && debug {
		fmt.Printf("[init] rmdir /.pivot_old: %v (ignored)\n", err)
	}

	if debug {
		fmt.Println("[init] pivot_root complete")
	}
	return nil
}
