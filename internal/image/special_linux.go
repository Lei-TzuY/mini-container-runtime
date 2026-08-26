//go:build linux

package image

import (
	"archive/tar"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	root, err := openExtractionRoot(destDir)
	if err != nil {
		return err
	}
	defer root.Close()
	parent, err := root.openParent(target, "special", true)
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
			return fmt.Errorf("refuse to replace directory %s with special node", target)
		}
		if err := unix.Unlinkat(parent.fd, parent.leaf, 0); err != nil {
			return fmt.Errorf("unlink existing special target %s: %w", target, err)
		}
	} else if !errors.Is(statErr, unix.ENOENT) {
		return fmt.Errorf("inspect special target %s: %w", target, statErr)
	}

	mode, dev, err := specialModeDevice(hdr)
	if err != nil {
		return err
	}
	if err := unix.Mknodat(parent.fd, parent.leaf, mode, int(dev)); err != nil {
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
