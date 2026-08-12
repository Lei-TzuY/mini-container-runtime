//go:build linux

package image

import (
	"archive/tar"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// makeSpecial creates device nodes (char/block) and FIFOs using mknod(2).
// Requires CAP_MKNOD (effectively root).  If the caller lacks the capability
// the syscall returns EPERM, which the caller logs as a warning.
func makeSpecial(target string, hdr *tar.Header) error {
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	_ = os.Remove(target) // mknod fails on existing path

	mode := uint32(hdr.FileInfo().Mode())
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
		// FIFOs have no device number.
	default:
		return fmt.Errorf("unexpected type flag: %d", hdr.Typeflag)
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
