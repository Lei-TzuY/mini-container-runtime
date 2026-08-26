//go:build linux

package image

import (
	"fmt"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
)

func restoreXattrsFD(fd int, target string, xattrs map[string][]byte) error {
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
		if err := unix.Fsetxattr(fd, name, xattrs[name], 0); err != nil {
			return fmt.Errorf("restore xattr %q on %s: %w", name, target, err)
		}
	}
	return nil
}
