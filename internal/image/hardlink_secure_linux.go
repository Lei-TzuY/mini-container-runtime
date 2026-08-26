//go:build linux

package image

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

func createHardlinkSecure(target, destDir, linkTarget string) error {
	return createHardlinkSecureWithHook(target, destDir, linkTarget, nil)
}

func createHardlinkSecureWithHook(target, destDir, linkTarget string, beforeLink func()) error {
	destAbs, err := filepath.Abs(destDir)
	if err != nil {
		return fmt.Errorf("resolve extraction root: %w", err)
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("resolve hardlink destination: %w", err)
	}
	linkAbs, err := filepath.Abs(linkTarget)
	if err != nil {
		return fmt.Errorf("resolve hardlink source: %w", err)
	}

	targetRel, err := hardlinkRelativePath(destAbs, targetAbs, "destination")
	if err != nil {
		return err
	}
	linkRel, err := hardlinkRelativePath(destAbs, linkAbs, "source")
	if err != nil {
		return err
	}

	rootFD, err := unix.Open(destAbs, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return fmt.Errorf("open extraction root %s: %w", destAbs, err)
	}
	defer unix.Close(rootFD)

	sourceParentFD, sourceOwnedFD, sourceLeaf, err := openHardlinkParent(rootFD, linkRel, false)
	if err != nil {
		return fmt.Errorf("open hardlink source parent: %w", err)
	}
	if sourceOwnedFD >= 0 {
		defer unix.Close(sourceOwnedFD)
	}

	destParentFD, destOwnedFD, destLeaf, err := openHardlinkParent(rootFD, targetRel, true)
	if err != nil {
		return fmt.Errorf("open hardlink destination parent: %w", err)
	}
	if destOwnedFD >= 0 {
		defer unix.Close(destOwnedFD)
	}

	if beforeLink != nil {
		beforeLink()
	}

	var sourceStat unix.Stat_t
	if err := unix.Fstatat(sourceParentFD, sourceLeaf, &sourceStat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return fmt.Errorf("inspect hardlink source %s: %w", linkTarget, err)
	}
	if sourceStat.Mode&unix.S_IFMT == unix.S_IFDIR {
		return fmt.Errorf("refuse hardlink to directory %s", linkTarget)
	}

	var destStat unix.Stat_t
	statErr := unix.Fstatat(destParentFD, destLeaf, &destStat, unix.AT_SYMLINK_NOFOLLOW)
	if statErr == nil {
		if destStat.Mode&unix.S_IFMT == unix.S_IFDIR {
			return fmt.Errorf("refuse to replace directory %s with hardlink", target)
		}
		if err := unix.Unlinkat(destParentFD, destLeaf, 0); err != nil {
			return fmt.Errorf("unlink existing hardlink destination %s: %w", target, err)
		}
	} else if !errors.Is(statErr, unix.ENOENT) {
		return fmt.Errorf("inspect hardlink destination %s: %w", target, statErr)
	}

	// flags=0 intentionally does not follow a symlink source leaf. Both source
	// and destination parent directories are pinned, so pathname replacement of
	// either archive parent cannot redirect this link operation outside destDir.
	if err := unix.Linkat(sourceParentFD, sourceLeaf, destParentFD, destLeaf, 0); err != nil {
		return fmt.Errorf("hardlink %s → %s relative to pinned parents: %w", target, linkTarget, err)
	}
	return nil
}

func hardlinkRelativePath(rootAbs, targetAbs, role string) (string, error) {
	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("hardlink %s path %q escapes extraction root %q", role, targetAbs, rootAbs)
	}
	return rel, nil
}

func openHardlinkParent(rootFD int, rel string, create bool) (parentFD, ownedFD int, leaf string, err error) {
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) == 0 {
		return -1, -1, "", fmt.Errorf("empty hardlink path")
	}

	parentFD, ownedFD = rootFD, -1
	for _, part := range parts[:len(parts)-1] {
		if part == "" || part == "." || part == ".." {
			if ownedFD >= 0 {
				_ = unix.Close(ownedFD)
			}
			return -1, -1, "", fmt.Errorf("invalid hardlink path component %q", part)
		}
		fd, openErr := unix.Openat(parentFD, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if create && errors.Is(openErr, unix.ENOENT) {
			if mkdirErr := unix.Mkdirat(parentFD, part, 0755); mkdirErr != nil && !errors.Is(mkdirErr, unix.EEXIST) {
				if ownedFD >= 0 {
					_ = unix.Close(ownedFD)
				}
				return -1, -1, "", fmt.Errorf("mkdir hardlink parent %q: %w", part, mkdirErr)
			}
			fd, openErr = unix.Openat(parentFD, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		}
		if openErr != nil {
			if ownedFD >= 0 {
				_ = unix.Close(ownedFD)
			}
			return -1, -1, "", fmt.Errorf("open hardlink parent %q without symlinks: %w", part, openErr)
		}
		if ownedFD >= 0 {
			_ = unix.Close(ownedFD)
		}
		ownedFD, parentFD = fd, fd
	}

	leaf = parts[len(parts)-1]
	if leaf == "" || leaf == "." || leaf == ".." {
		if ownedFD >= 0 {
			_ = unix.Close(ownedFD)
		}
		return -1, -1, "", fmt.Errorf("invalid hardlink leaf %q", leaf)
	}
	return parentFD, ownedFD, leaf, nil
}
