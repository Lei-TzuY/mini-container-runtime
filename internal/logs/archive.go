package logs

import (
	"fmt"
	"os"
	"path/filepath"
)

var archiveLstat = os.Lstat
var archiveSyncDir = syncArchiveDirectory

func syncArchiveDirectory(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open log archive directory %q for fsync: %w", dir, err)
	}
	defer f.Close()
	if err := f.Sync(); err != nil {
		return fmt.Errorf("fsync log archive directory %q: %w", dir, err)
	}
	return nil
}

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
	nlink, err := fileInfoLinkCount(fi)
	if err != nil {
		return nil, false, fmt.Errorf("inspect archived log link count %q: %w", p, err)
	}
	if nlink != 1 {
		return nil, false, fmt.Errorf("unsafe archived log path %q: link count %d", p, nlink)
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
	nlink, err := fileInfoLinkCount(current)
	if err != nil {
		return fmt.Errorf("revalidate archived log link count %q: %w", p, err)
	}
	if nlink != 1 {
		return fmt.Errorf("archived log path %q gained hard links before rotation (link count %d)", p, nlink)
	}
	return nil
}

// ArchiveLogFile shifts old log files (e.g. log.1 -> log.2) up to maxFiles.
func ArchiveLogFile(logPath string, maxFiles int) error {
	if maxFiles <= 1 {
		return nil
	}

	dir := filepath.Dir(logPath)
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
				if err := archiveSyncDir(dir); err != nil {
					return fmt.Errorf("persist expired archived log removal %q: %w", src, err)
				}
			} else {
				if err := os.Rename(src, dst); err != nil {
					return fmt.Errorf("rotate archived log %q to %q: %w", src, dst, err)
				}
				if err := archiveSyncDir(dir); err != nil {
					return fmt.Errorf("persist archived log rotation %q to %q: %w", src, dst, err)
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
		if err := archiveSyncDir(dir); err != nil {
			return fmt.Errorf("persist active log archive %q to %q: %w", logPath, dst, err)
		}
	}

	return nil
}
