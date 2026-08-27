//go:build linux

package image

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

type hardlinkAtFunc func(olddirfd int, oldpath string, newdirfd int, newpath string, flags int) error

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

	// Pin the exact source inode before any destructive destination work. A
	// parent dirfd alone is insufficient: another actor can replace sourceLeaf
	// between an Fstatat proof and a later Linkat by pathname.
	sourceFD, err := unix.Openat(sourceParent.fd, sourceParent.leaf, unix.O_PATH|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return fmt.Errorf("pin hardlink source %s: %w", linkTarget, err)
	}
	defer unix.Close(sourceFD)

	var sourceStat unix.Stat_t
	if err := unix.Fstat(sourceFD, &sourceStat); err != nil {
		return fmt.Errorf("inspect pinned hardlink source %s: %w", linkTarget, err)
	}
	if sourceStat.Mode&unix.S_IFMT == unix.S_IFDIR {
		return fmt.Errorf("refuse hardlink to directory %s", linkTarget)
	}

	if beforeLink != nil {
		beforeLink()
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

	if err := linkPinnedHardlinkSource(sourceFD, sourceStat.Mode, destParent.fd, destParent.leaf, unix.Linkat); err != nil {
		return fmt.Errorf("hardlink pinned source %s → %s: %w", target, linkTarget, err)
	}
	return nil
}

func linkPinnedHardlinkSource(sourceFD int, sourceMode uint32, destParentFD int, destLeaf string, linkat hardlinkAtFunc) error {
	if linkat == nil {
		return fmt.Errorf("hardlink operation is nil")
	}

	// AT_EMPTY_PATH is the only fd-native link operation here: it names the
	// exact inode already proven above, including when that inode is itself a
	// symlink. It can require CAP_DAC_READ_SEARCH, so ordinary rootless callers
	// may need the procfs fallback below for non-symlink inodes.
	if err := linkat(sourceFD, "", destParentFD, destLeaf, unix.AT_EMPTY_PATH); err == nil {
		return nil
	} else if !errors.Is(err, unix.EPERM) && !errors.Is(err, unix.EINVAL) && !errors.Is(err, unix.ENOENT) {
		return err
	} else if sourceMode&unix.S_IFMT == unix.S_IFLNK {
		// /proc/self/fd/<fd> is itself a symlink. Using AT_SYMLINK_FOLLOW on
		// that path is a valid AT_EMPTY_PATH substitute for ordinary files, but
		// for an O_PATH|O_NOFOLLOW descriptor that pins a symlink inode it would
		// continue through the pinned symlink and link its target instead. That
		// silently changes archive semantics, so fail closed rather than
		// dereference a hardlink-to-symlink source.
		return fmt.Errorf("fd-native hardlink for symlink source unavailable: %w", err)
	}

	procSource := fmt.Sprintf("/proc/self/fd/%d", sourceFD)
	if err := linkat(unix.AT_FDCWD, procSource, destParentFD, destLeaf, unix.AT_SYMLINK_FOLLOW); err != nil {
		return fmt.Errorf("link pinned source via fd path: %w", err)
	}
	return nil
}
