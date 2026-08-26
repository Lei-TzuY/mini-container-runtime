//go:build linux

package image

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

func writeRegularSecure(target, destDir string, hdr *tar.Header, r io.Reader) error {
	return writeRegularSecureWithHook(target, destDir, hdr, r, nil)
}

func writeRegularSecureWithHook(target, destDir string, hdr *tar.Header, r io.Reader, beforeCreate func()) error {
	destAbs, err := filepath.Abs(destDir)
	if err != nil {
		return fmt.Errorf("resolve extraction root: %w", err)
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("resolve regular target: %w", err)
	}
	rel, err := filepath.Rel(destAbs, targetAbs)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path traversal detected: %q escapes %q", target, destDir)
	}

	rootFD, err := unix.Open(destAbs, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return fmt.Errorf("open extraction root %s: %w", destAbs, err)
	}
	defer unix.Close(rootFD)

	parentFD := rootFD
	ownedParentFD := -1
	parts := strings.Split(rel, string(filepath.Separator))
	for _, part := range parts[:len(parts)-1] {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("invalid regular extraction path component %q", part)
		}
		fd, openErr := unix.Openat(parentFD, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if errors.Is(openErr, unix.ENOENT) {
			if mkdirErr := unix.Mkdirat(parentFD, part, 0755); mkdirErr != nil && !errors.Is(mkdirErr, unix.EEXIST) {
				return fmt.Errorf("mkdir extraction parent %q: %w", part, mkdirErr)
			}
			fd, openErr = unix.Openat(parentFD, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		}
		if openErr != nil {
			return fmt.Errorf("open extraction parent %q without symlinks: %w", part, openErr)
		}
		if ownedParentFD >= 0 {
			_ = unix.Close(ownedParentFD)
		}
		ownedParentFD = fd
		parentFD = fd
	}
	if ownedParentFD >= 0 {
		defer unix.Close(ownedParentFD)
	}

	leaf := parts[len(parts)-1]
	if leaf == "" || leaf == "." || leaf == ".." {
		return fmt.Errorf("invalid regular extraction leaf %q", leaf)
	}

	if beforeCreate != nil {
		beforeCreate()
	}

	var st unix.Stat_t
	statErr := unix.Fstatat(parentFD, leaf, &st, unix.AT_SYMLINK_NOFOLLOW)
	if statErr == nil {
		if st.Mode&unix.S_IFMT == unix.S_IFDIR {
			return fmt.Errorf("refuse to replace directory %s with regular file", target)
		}
		if err := unix.Unlinkat(parentFD, leaf, 0); err != nil {
			return fmt.Errorf("unlink existing regular target %s: %w", target, err)
		}
	} else if !errors.Is(statErr, unix.ENOENT) {
		return fmt.Errorf("inspect regular target %s: %w", target, statErr)
	}

	fd, err := unix.Openat(parentFD, leaf, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, uint32(hdr.FileInfo().Mode().Perm()))
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
