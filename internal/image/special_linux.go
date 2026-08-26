//go:build linux

package image

import (
	"archive/tar"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// makeSpecialSecure creates device nodes and FIFOs relative to a pinned
// extraction parent. Parent traversal uses O_NOFOLLOW so a concurrent
// rename/symlink replacement cannot redirect a privileged mknod outside the
// extraction root.
func makeSpecialSecure(target, destDir string, hdr *tar.Header) error {
	return makeSpecialSecureWithHook(target, destDir, hdr, nil)
}

func makeSpecialSecureWithHook(target, destDir string, hdr *tar.Header, beforeCreate func()) error {
	destAbs, err := filepath.Abs(destDir)
	if err != nil {
		return fmt.Errorf("resolve extraction root: %w", err)
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("resolve special target: %w", err)
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

	parentFD, ownedParentFD := rootFD, -1
	parts := strings.Split(rel, string(filepath.Separator))
	for _, part := range parts[:len(parts)-1] {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("invalid special extraction path component %q", part)
		}
		fd, openErr := unix.Openat(parentFD, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if errors.Is(openErr, unix.ENOENT) {
			if mkdirErr := unix.Mkdirat(parentFD, part, 0o755); mkdirErr != nil && !errors.Is(mkdirErr, unix.EEXIST) {
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
		ownedParentFD, parentFD = fd, fd
	}
	if ownedParentFD >= 0 {
		defer unix.Close(ownedParentFD)
	}

	leaf := parts[len(parts)-1]
	if leaf == "" || leaf == "." || leaf == ".." {
		return fmt.Errorf("invalid special extraction leaf %q", leaf)
	}
	if beforeCreate != nil {
		beforeCreate()
	}

	var st unix.Stat_t
	statErr := unix.Fstatat(parentFD, leaf, &st, unix.AT_SYMLINK_NOFOLLOW)
	if statErr == nil {
		if st.Mode&unix.S_IFMT == unix.S_IFDIR {
			return fmt.Errorf("refuse to replace directory %s with special node", target)
		}
		if err := unix.Unlinkat(parentFD, leaf, 0); err != nil {
			return fmt.Errorf("unlink existing special target %s: %w", target, err)
		}
	} else if !errors.Is(statErr, unix.ENOENT) {
		return fmt.Errorf("inspect special target %s: %w", target, statErr)
	}

	mode, dev, err := specialModeDevice(hdr)
	if err != nil {
		return err
	}
	if err := unix.Mknodat(parentFD, leaf, mode, int(dev)); err != nil {
		return err
	}
	return nil
}

func specialModeDevice(hdr *tar.Header) (uint32, uint64, error) {
	mode := uint32(hdr.FileInfo().Mode().Perm())
	var dev uint64
	switch hdr.Typeflag {
	case tar.TypeChar:
		mode |= syscall.S_IFCHR
		dev = mkdev(uint(hdr.Devmajor), uint(hdr.Devminor))
	case tar.TypeBlock:
		mode |= syscall.S_IFBLK
		dev = mkdev(uint(hdr.Devmajor), uint(hdr.Devminor))
	case tar.TypeFifo:
		mode |= syscall.S_IFIFO
	default:
		return 0, 0, fmt.Errorf("unexpected type flag: %d", hdr.Typeflag)
	}
	return mode, dev, nil
}

// makeSpecial remains available as the portable pathname primitive for callers
// outside archive extraction. Production tar extraction uses makeSpecialSecure.
func makeSpecial(target string, hdr *tar.Header) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	_ = os.Remove(target)
	mode, dev, err := specialModeDevice(hdr)
	if err != nil {
		return err
	}
	return syscall.Mknod(target, mode, int(dev))
}

// mkdev encodes a major/minor pair into a Linux device number.
// The encoding is: bits[19:8]=major, bits[7:0]=minor_low, bits[31:20]=minor_high.
func mkdev(major, minor uint) uint64 {
	return (uint64(major) << 8) |
		(uint64(minor) & 0xff) |
		((uint64(minor) &^ 0xff) << 12)
}
