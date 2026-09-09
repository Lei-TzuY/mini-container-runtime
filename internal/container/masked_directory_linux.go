//go:build linux

package container

import (
	"fmt"
	"path/filepath"
	"syscall"
)

// mountMaskedDirectory hides an existing directory behind an empty, read-only
// tmpfs. It is the Linux primitive used for OCI linux.maskedPaths directory
// targets; callers must resolve the target inside the container rootfs before
// invoking it.
func mountMaskedDirectory(target string) error {
	if target == "" || !filepath.IsAbs(target) || filepath.Clean(target) != target {
		return fmt.Errorf("masked directory target %q must be a canonical absolute path", target)
	}

	flags := uintptr(syscall.MS_RDONLY | syscall.MS_NOSUID | syscall.MS_NODEV | syscall.MS_NOEXEC)
	if err := syscall.Mount("tmpfs", target, "tmpfs", flags, "mode=000,size=0"); err != nil {
		return fmt.Errorf("mount empty masked-directory tmpfs at %s: %w", target, err)
	}
	return nil
}
