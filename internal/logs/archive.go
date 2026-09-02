package logs

import (
	"fmt"
	"os"
)

var archiveLstat = os.Lstat
var archiveSyncDir = func(string) error { return nil }

func inspectArchiveFile(p string) (os.FileInfo, bool, error) {
	fi, err := archiveLstat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("inspect archived log %q: %w", p, err)
	}
	if !fi.Mode().IsRegular() {
		return nil, false, fmt.Errorf("unsafe archived log path %q: mode %v", p, fi.Mode())
	}
	return fi, true, nil
}

func revalidateArchiveFile(p string, inspected os.FileInfo) error {
	current, err := archiveLstat(p)
	if err != nil {
		return fmt.Errorf("revalidate archived log %q: %w", p, err)
	}
	if !current.Mode().IsRegular() {
		return fmt.Errorf("unsafe archived log path %q during revalidation: mode %v", p, current.Mode())
	}
	if !os.SameFile(inspected, current) {
		return fmt.Errorf("archived log path %q changed identity before rotation", p)
	}
	return nil
}

// ArchiveLogFile shifts old log files (e.g. log.1 -> log.2) up to maxFiles.
func ArchiveLogFile(logPath string, maxFiles int) error {
	if maxFiles <= 1 {
		return nil
	}

	for i := maxFiles - 1; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", logPath, i)
		dst := fmt.Sprintf("%s.%d", logPath, i+1)
		inspected, exists, err := inspectArchiveFile(src)
		if err != nil {
			return err
		}
		if exists {
			if err := revalidateArchiveFile(src, inspected); err != nil {
				return err
			}
			if i+1 >= maxFiles {
				if err := os.Remove(src); err != nil {
					return fmt.Errorf("remove expired archived log %q: %w", src, err)
				}
			} else {
				if err := os.Rename(src, dst); err != nil {
					return fmt.Errorf("rotate archived log %q to %q: %w", src, dst, err)
				}
			}
		}
	}

	inspected, exists, err := inspectArchiveFile(logPath)
	if err != nil {
		return err
	}
	if exists {
		if err := revalidateArchiveFile(logPath, inspected); err != nil {
			return err
		}
		dst := fmt.Sprintf("%s.1", logPath)
		if err := os.Rename(logPath, dst); err != nil {
			return fmt.Errorf("archive active log %q to %q: %w", logPath, dst, err)
		}
	}

	return nil
}
