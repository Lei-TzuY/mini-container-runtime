//go:build !linux

package image

import (
	"archive/tar"
	"fmt"
)

// makeSpecial is a no-op stub on non-Linux platforms.
// Device nodes and FIFOs require Linux mknod(2) and are skipped silently.
func makeSpecial(_ string, hdr *tar.Header) error {
	return fmt.Errorf("device node type %d not supported on this platform", hdr.Typeflag)
}
