//go:build linux

package container

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func consumePinnedRootFSPath(fallback string) (string, error) {
	path, present := os.LookupEnv(pinnedRootFSPathEnvKey)
	if !present {
		return fallback, nil
	}
	if err := os.Unsetenv(pinnedRootFSPathEnvKey); err != nil {
		return "", fmt.Errorf("clear pinned rootfs path environment: %w", err)
	}
	if path == "" {
		return "", fmt.Errorf("pinned rootfs path marker is empty")
	}
	return path, nil
}

// attachPinnedRootFS reopens the admitted pathname from inside the child's
// mount namespace, proves that it still names the directory pinned by the
// parent, then clones and attaches that child-local mount tree.
//
// A descriptor inherited from the parent retains the parent's mount object and
// cannot itself be used as a mount target/source in the child's namespace.
// Reopening without the identity comparison would reintroduce a pathname
// replacement race; comparing the already-open objects before open_tree keeps
// the operation fail-closed.
func attachPinnedRootFS(pinnedSource, admittedPath, target string) error {
	if pinnedSource == "" || admittedPath == "" || target == "" {
		return fmt.Errorf("rootfs pinned source, admitted path, and target must be non-empty")
	}

	pinnedFD, err := unix.Open(pinnedSource, unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open inherited pinned rootfs: %w", err)
	}
	defer unix.Close(pinnedFD)

	currentFD, err := unix.Open(admittedPath, unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("reopen admitted rootfs in child namespace: %w", err)
	}
	defer unix.Close(currentFD)

	var pinnedStat, currentStat unix.Stat_t
	if err := unix.Fstat(pinnedFD, &pinnedStat); err != nil {
		return fmt.Errorf("stat inherited pinned rootfs: %w", err)
	}
	if err := unix.Fstat(currentFD, &currentStat); err != nil {
		return fmt.Errorf("stat child-local admitted rootfs: %w", err)
	}
	if pinnedStat.Dev != currentStat.Dev || pinnedStat.Ino != currentStat.Ino {
		return fmt.Errorf(
			"admitted rootfs identity changed before child mount attachment: pinned dev=%d ino=%d, current dev=%d ino=%d",
			pinnedStat.Dev,
			pinnedStat.Ino,
			currentStat.Dev,
			currentStat.Ino,
		)
	}

	treeFlags := uint(unix.OPEN_TREE_CLONE | unix.AT_EMPTY_PATH | unix.O_CLOEXEC)
	treeFD, err := unix.OpenTree(currentFD, "", treeFlags)
	if err != nil {
		return fmt.Errorf("clone child-local rootfs mount: %w", err)
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
