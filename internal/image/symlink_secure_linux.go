//go:build linux

package image

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

func createSymlinkSecure(target, destDir, linkname string) error {
	return createSymlinkSecureWithHook(target, destDir, linkname, nil)
}

func createSymlinkSecureWithHook(target, destDir, linkname string, beforeCreate func()) error {
	root, err := openExtractionRoot(destDir)
	if err != nil {
		return err
	}
	defer root.Close()
	parent, err := root.openParent(target, "symlink", true)
	if err != nil {
		return err
	}
	defer parent.Close()
	if beforeCreate != nil {
		beforeCreate()
	}

	var st unix.Stat_t
	statErr := unix.Fstatat(parent.fd, parent.leaf, &st, unix.AT_SYMLINK_NOFOLLOW)
	if statErr == nil {
		if st.Mode&unix.S_IFMT == unix.S_IFDIR {
			return fmt.Errorf("refuse to replace directory %s with symlink", target)
		}
		if err := unix.Unlinkat(parent.fd, parent.leaf, 0); err != nil {
			return fmt.Errorf("unlink existing symlink target %s: %w", target, err)
		}
	} else if !errors.Is(statErr, unix.ENOENT) {
		return fmt.Errorf("inspect symlink target %s: %w", target, statErr)
	}

	if err := unix.Symlinkat(linkname, parent.fd, parent.leaf); err != nil {
		return fmt.Errorf("symlink %s → %s relative to pinned parent: %w", target, linkname, err)
	}
	return nil
}
