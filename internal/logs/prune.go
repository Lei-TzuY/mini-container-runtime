package logs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var pruneBeforeInfo = func(string) {}
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
		if strings.Contains(name, ".log.") && !entry.IsDir() {
			path := filepath.Join(logDir, name)
			pruneBeforeInfo(path)
			fi, err := entry.Info()
			if err != nil {
				return deletedCount, fmt.Errorf("inspect rotated log %q: %w", path, err)
			}
			if fi.ModTime().Before(cutoff) {
				pruneBeforeDelete(path)
				if err := removeExpiredArchive(path, fi); err != nil {
					return deletedCount, fmt.Errorf("prune rotated log %q: %w", path, err)
				}
				if err := archiveSyncDir(logDir); err != nil {
					return deletedCount, fmt.Errorf("persist pruned rotated log removal %q: %w", path, err)
				}
				deletedCount++
			}
		}
	}

	return deletedCount, nil
}
