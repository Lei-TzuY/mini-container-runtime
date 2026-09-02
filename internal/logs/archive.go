package logs

import (
	"fmt"
	"os"
)

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

// ArchiveLogFile shifts old log files (e.g. log.1 -> log.2) up to maxFiles.
func ArchiveLogFile(logPath string, maxFiles int) error {
	if maxFiles <= 1 {
		return nil
	}

	for i := maxFiles - 1; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", logPath, i)
		dst := fmt.Sprintf("%s.%d", logPath, i+1)
		if fileExists(src) {
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

	if fileExists(logPath) {
		dst := fmt.Sprintf("%s.1", logPath)
		if err := os.Rename(logPath, dst); err != nil {
			return fmt.Errorf("archive active log %q to %q: %w", logPath, dst, err)
		}
	}

	return nil
}
