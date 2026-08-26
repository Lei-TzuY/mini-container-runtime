//go:build linux

package image

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

func createHardlinkSecure(target, destDir, linkTarget string) error {
	return createHardlinkSecureWithHook(target, destDir, linkTarget, nil)
}

func createHardlinkSecureWithHook(target, destDir, linkTarget string, beforeLink func()) error {
	root, err := openExtractionRoot(destDir)
	if err != nil {
		return err
	}
	defer root.Close()

	sourceParent, err := root.openParent(linkTarget, "hardlink source", false)
	if err != nil {
		return fmt.Errorf("open hardlink source parent: %w", err)
	}
	defer sourceParent.Close()

	destParent, err := root.openParent(target, "hardlink destination", true)
	if err != nil {
		return fmt.Errorf("open hardlink destination parent: %w", err)
	}
	defer destParent.Close()

	if beforeLink != nil {
		beforeLink()
	}

	var sourceStat unix.Stat_t
	if err := unix.Fstatat(sourceParent.fd, sourceParent.leaf, &sourceStat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return fmt.Errorf("inspect hardlink source %s: %w", linkTarget, err)
	}
	if sourceStat.Mode&unix.S_IFMT == unix.S_IFDIR {
		return fmt.Errorf("refuse hardlink to directory %s", linkTarget)
	}

	var destStat unix.Stat_t
	statErr := unix.Fstatat(destParent.fd, destParent.leaf, &destStat, unix.AT_SYMLINK_NOFOLLOW)
	if statErr == nil {
		if destStat.Mode&unix.S_IFMT == unix.S_IFDIR {
			return fmt.Errorf("refuse to replace directory %s with hardlink", target)
		}
		if err := unix.Unlinkat(destParent.fd, destParent.leaf, 0); err != nil {
			return fmt.Errorf("unlink existing hardlink destination %s: %w", target, err)
		}
	} else if !errors.Is(statErr, unix.ENOENT) {
		return fmt.Errorf("inspect hardlink destination %s: %w", target, statErr)
	}

	// flags=0 intentionally does not follow a symlink source leaf. Both source
	// and destination parent directories share one pinned extraction-root
	// generation, so pathname replacement cannot redirect either side.
	if err := unix.Linkat(sourceParent.fd, sourceParent.leaf, destParent.fd, destParent.leaf, 0); err != nil {
		return fmt.Errorf("hardlink %s → %s relative to pinned parents: %w", target, linkTarget, err)
	}
	return nil
}
