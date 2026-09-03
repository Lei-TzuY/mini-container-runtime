package logs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var pruneBeforeDelete = func(string) {}

// PruneRotatedLogs deletes rotated log files older than maxAge duration.
func PruneRotatedLogs(logDir string, maxAge time.Duration) (int, error) {
	if maxAge <= 0 {
		return 0, nil
	}

	entries, err := os.ReadDir(logDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read log dir: %w", err)
	}

	cutoff := time.Now().Add(-maxAge)
	deletedCount := 0

	for _, entry := range entries {
		name := entry.Name()
		if (strings.Contains(name, ".log.") || strings.HasSuffix(name, ".gz")) && !entry.IsDir() {
			path := filepath.Join(logDir, name)
			fi, err := entry.Info()
			if err == nil && fi.ModTime().Before(cutoff) {
				pruneBeforeDelete(path)
				if err := os.Remove(path); err == nil {
					deletedCount++
				}
			}
		}
	}

	return deletedCount, nil
}
