package logs

import (
	"fmt"
	"os"
)

func archiveFileExists(p string) (bool, error) {
	fi, err := os.Lstat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("inspect archived log %q: %w", p, err)
	}
	if !fi.Mode().IsRegular() {
		return false, fmt.Errorf("unsafe archived log path %q: mode %v", p, fi.Mode())
	}
	return true, nil
}

// ArchiveLogFile shifts old log files (e.g. log.1 -> log.2) up to maxFiles.
func ArchiveLogFile(logPath string, maxFiles int) error {
	if maxFiles <= 1 {
		return nil
	}

	for i := maxFiles - 1; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", logPath, i)
		dst := fmt.Sprintf("%s.%d", logPath, i+1)
		exists, err := archiveFileExists(src)
		if err != nil {
			return err
		}
		if exists {
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

	exists, err := archiveFileExists(logPath)
	if err != nil {
		return err
	}
	if exists {
		dst := fmt.Sprintf("%s.1", logPath)
		if err := os.Rename(logPath, dst); err != nil {
			return fmt.Errorf("archive active log %q to %q: %w", logPath, dst, err)
		}
	}

	return nil
}
