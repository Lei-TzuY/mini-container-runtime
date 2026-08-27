//go:build linux

package image

import (
	"fmt"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
)

func restoreXattrsWith(target string, xattrs map[string][]byte, set func(name string, value []byte) error) error {
	if len(xattrs) == 0 {
		return nil
	}
	names := make([]string, 0, len(xattrs))
	for name := range xattrs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if strings.IndexByte(name, 0) >= 0 {
			return fmt.Errorf("restore xattr on %s: invalid NUL in name", target)
		}
		if err := set(name, xattrs[name]); err != nil {
			return fmt.Errorf("restore xattr %q on %s: %w", name, target, err)
		}
	}
	return nil
}

func restoreXattrsFD(fd int, target string, xattrs map[string][]byte) error {
	return restoreXattrsWith(target, xattrs, func(name string, value []byte) error {
		return unix.Fsetxattr(fd, name, value, 0)
	})
}

// restoreXattrsPinnedFD restores xattrs through the kernel-owned procfs path
// for an already-pinned O_PATH descriptor. Fsetxattr rejects O_PATH FDs, while
// /proc/self/fd/<n> resolves to that exact inode rather than the archive
// pathname, so a concurrent rename/replacement cannot redirect metadata writes.
func restoreXattrsPinnedFD(fd int, target string, xattrs map[string][]byte) error {
	fdPath := fmt.Sprintf("/proc/self/fd/%d", fd)
	return restoreXattrsWith(target, xattrs, func(name string, value []byte) error {
		return unix.Setxattr(fdPath, name, value, 0)
	})
}
