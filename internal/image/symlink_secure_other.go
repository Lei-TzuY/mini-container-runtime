//go:build !linux

package image

import (
	"fmt"
	"os"
)

func createSymlinkSecure(target, destDir, linkname string) error {
	_ = destDir
	if err := os.RemoveAll(target); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove existing node before symlink %s: %w", target, err)
	}
	if err := os.Symlink(linkname, target); err != nil && !os.IsExist(err) {
		return fmt.Errorf("symlink %s → %s: %w", target, linkname, err)
	}
	return nil
}
