//go:build linux

package container

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// attachPinnedRootFS clones the admitted rootfs mount referenced by source and
// attaches that clone to a directory in the container's mount namespace.
//
// The source descriptor is inherited from the parent namespace. Legacy
// mount(MS_BIND) cannot attach a mount reached through that foreign mount
// object. open_tree(2) creates a detached clone and move_mount(2) attaches it
// through an already-open target directory, preserving both the pinned-inode
// TOCTOU boundary and the child mount-namespace boundary.
func attachPinnedRootFS(source, target string) error {
	if source == "" || target == "" {
		return fmt.Errorf("rootfs source and target must be non-empty")
	}

	sourceFD, err := unix.Open(source, unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open pinned rootfs: %w", err)
	}
	defer unix.Close(sourceFD)

	treeFlags := uint(unix.OPEN_TREE_CLONE | unix.AT_EMPTY_PATH | unix.O_CLOEXEC)
	treeFD, err := unix.OpenTree(sourceFD, "", treeFlags)
	if err != nil {
		return fmt.Errorf("clone pinned rootfs mount: %w", err)
	}
	defer unix.Close(treeFD)

	targetFD, err := unix.Open(target, unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open staged rootfs target: %w", err)
	}
	defer unix.Close(targetFD)

	moveFlags := unix.MOVE_MOUNT_F_EMPTY_PATH | unix.MOVE_MOUNT_T_EMPTY_PATH
	if err := unix.MoveMount(treeFD, "", targetFD, "", moveFlags); err != nil {
		return fmt.Errorf("attach cloned rootfs mount: %w", err)
	}
	return nil
}
