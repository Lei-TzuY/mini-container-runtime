//go:build linux

// internal/rootfs/rootfs_linux.go
//
// Filesystem Isolation — pivot_root and chroot
// ─────────────────────────────────────────────
// Container filesystem isolation means the container process sees a
// completely different directory tree than the host.  This is achieved by
// changing what "/" means for that process.
//
// Two mechanisms:
//
//   chroot(2)     – simple, widely available, but leaves the host's mount
//                   table visible inside the container (processes can escape
//                   with a crafted sequence of chdir + chroot calls if they
//                   have sufficient privilege).
//
//   pivot_root(2) – proper, runc/Docker default.  Atomically swaps the
//                   root filesystem and then lets us unmount the old one,
//                   so the host's mounts are not visible at all.
//
// We try pivot_root first and fall back to chroot if it fails (e.g., the
// root filesystem is not a mount point).
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

// Isolate changes the root filesystem of the current process to newRoot.
// It must be called after the process has entered a new mount namespace
// (CLONE_NEWNS), otherwise the bind-mount in step 1 would affect the host.
func Isolate(newRoot string, debug bool) error {
	if err := pivotRoot(newRoot, debug); err != nil {
		if debug {
			fmt.Printf("[init] pivot_root failed (%v), falling back to chroot\n", err)
		}
		return chrootIsolate(newRoot, debug)
	}
	return nil
}

// pivotRoot implements filesystem isolation using the pivot_root(2) syscall.
// This is the technique used by runc and Docker's default runtime.
func pivotRoot(newRoot string, debug bool) error {
	// Step 1: bind-mount newRoot onto itself.
	//
	// pivot_root(2) requires newRoot to already be a mount point.  A plain
	// directory is NOT a mount point.  Bind-mounting the directory onto
	// itself promotes it to a mount point without changing its contents.
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
	// This is the correct way to unmount a potentially busy mount.
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

// chrootIsolate is the simpler fallback using chroot(2).
// Less secure than pivot_root because the host's mount table is still
// partially visible, but sufficient for many educational scenarios and
// environments that don't support pivot_root (e.g. some VMs).
func chrootIsolate(newRoot string, debug bool) error {
	if err := syscall.Chroot(newRoot); err != nil {
		return fmt.Errorf("chroot(%s): %w", newRoot, err)
	}
	// After chroot the kernel has changed "/" but not the CWD.
	// We must chdir into the new root explicitly.
	if err := syscall.Chdir("/"); err != nil {
		return fmt.Errorf("chdir / after chroot: %w", err)
	}
	if debug {
		fmt.Println("[init] chroot complete")
	}
	return nil
}
