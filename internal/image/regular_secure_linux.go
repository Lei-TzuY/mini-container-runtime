//go:build linux

package image

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

func writeRegularSecure(target, destDir string, hdr *tar.Header, r io.Reader) error {
	return writeRegularSecureWithHook(target, destDir, hdr, r, nil)
}

func writeRegularSecureWithHook(target, destDir string, hdr *tar.Header, r io.Reader, beforeCreate func()) error {
	root, err := openExtractionRoot(destDir)
	if err != nil {
		return err
	}
	defer root.Close()
	parent, err := root.openParent(target, "regular", true)
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
			return fmt.Errorf("refuse to replace directory %s with regular file", target)
		}
		if err := unix.Unlinkat(parent.fd, parent.leaf, 0); err != nil {
			return fmt.Errorf("unlink existing regular target %s: %w", target, err)
		}
	} else if !errors.Is(statErr, unix.ENOENT) {
		return fmt.Errorf("inspect regular target %s: %w", target, statErr)
	}
	fd, err := unix.Openat(parent.fd, parent.leaf, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, uint32(hdr.FileInfo().Mode().Perm()))
	if err != nil {
		return fmt.Errorf("create %s relative to pinned parent: %w", target, err)
	}
	out := os.NewFile(uintptr(fd), target)
	if out == nil {
		_ = unix.Close(fd)
		return fmt.Errorf("wrap regular target fd for %s", target)
	}
	if _, err := io.Copy(out, r); err != nil {
		_ = out.Close()
		return fmt.Errorf("write %s: %w", target, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close %s: %w", target, err)
	}
	return nil
}

// writeRegular remains the narrow exclusive-create primitive covered by legacy
// unit tests. Production extraction uses writeRegularSecure above.
func writeRegular(target string, hdr *tar.Header, r io.Reader) error {
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, hdr.FileInfo().Mode())
	if err != nil {
		return fmt.Errorf("create %s exclusively: %w", target, err)
	}
	if _, err := io.Copy(out, r); err != nil {
		_ = out.Close()
		return fmt.Errorf("write %s: %w", target, err)
	}
	return out.Close()
}
