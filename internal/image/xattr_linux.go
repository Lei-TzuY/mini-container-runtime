//go:build linux

package image

import (
	"archive/tar"
	"fmt"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
)

const paxSchilyXattrPrefix = "SCHILY.xattr."

func tarXattrs(hdr *tar.Header) map[string][]byte {
	if hdr == nil || len(hdr.PAXRecords) == 0 {
		return nil
	}
	out := make(map[string][]byte)
	for key, value := range hdr.PAXRecords {
		if !strings.HasPrefix(key, paxSchilyXattrPrefix) {
			continue
		}
		name := strings.TrimPrefix(key, paxSchilyXattrPrefix)
		if name == "" {
			continue
		}
		out[name] = []byte(value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

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
