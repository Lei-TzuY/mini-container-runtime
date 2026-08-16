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
				_ = os.Remove(src)
			} else {
				_ = os.Rename(src, dst)
			}
		}
	}

	if fileExists(logPath) {
		_ = os.Rename(logPath, fmt.Sprintf("%s.1", logPath))
	}

	return nil
}
