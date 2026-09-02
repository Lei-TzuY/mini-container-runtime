package logs

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

var (
	compressArchiveRemove    = os.Remove
	compressArchiveGzipClose = func(w *gzip.Writer) error { return w.Close() }
	compressArchiveSync      = func(f *os.File) error { return f.Sync() }
	compressArchiveFileClose = func(f *os.File) error { return f.Close() }
	compressArchiveSyncDir   = syncArchiveDirectory
)

// CompressRotatedLog compresses logPath to logPath.gz and removes the uncompressed file.
func CompressRotatedLog(logPath string) error {
	srcFile, err := os.OpenFile(logPath, os.O_RDONLY|unix.O_NOFOLLOW, 0)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("open log file: %w", err)
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("stat opened log file %q: %w", logPath, err)
	}
	if !srcInfo.Mode().IsRegular() {
		return fmt.Errorf("unsafe compressed source log %q: mode %v", logPath, srcInfo.Mode())
	}

	gzPath := logPath + ".gz"
	dstFile, err := os.OpenFile(gzPath, os.O_WRONLY|os.O_CREATE|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0644)
	if err != nil {
		return fmt.Errorf("create gz file: %w", err)
	}
	defer dstFile.Close()

	dstInfo, err := dstFile.Stat()
	if err != nil {
		return fmt.Errorf("stat opened gzip archive %q: %w", gzPath, err)
	}
	if !dstInfo.Mode().IsRegular() {
		return fmt.Errorf("unsafe gzip archive destination %q: mode %v", gzPath, dstInfo.Mode())
	}
	var dstStat unix.Stat_t
	if err := unix.Fstat(int(dstFile.Fd()), &dstStat); err != nil {
		return fmt.Errorf("fstat gzip archive %q: %w", gzPath, err)
	}
	if dstStat.Nlink != 1 {
		return fmt.Errorf("unsafe gzip archive destination %q: link count %d", gzPath, dstStat.Nlink)
	}
	if err := dstFile.Truncate(0); err != nil {
		return fmt.Errorf("truncate gzip archive %q: %w", gzPath, err)
	}
	if _, err := dstFile.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind gzip archive %q: %w", gzPath, err)
	}

	gzWriter := gzip.NewWriter(dstFile)

	if _, err := io.Copy(gzWriter, srcFile); err != nil {
		_ = gzWriter.Close()
		return fmt.Errorf("gzip compress: %w", err)
	}
	if err := compressArchiveGzipClose(gzWriter); err != nil {
		return fmt.Errorf("finalize gzip archive %q: %w", gzPath, err)
	}
	if err := compressArchiveSync(dstFile); err != nil {
		return fmt.Errorf("sync gzip archive %q: %w", gzPath, err)
	}
	if err := compressArchiveFileClose(dstFile); err != nil {
		return fmt.Errorf("close gzip archive %q: %w", gzPath, err)
	}

	currentDstInfo, err := os.Lstat(gzPath)
	if err != nil {
		return fmt.Errorf("revalidate gzip archive %q: %w", gzPath, err)
	}
	if !currentDstInfo.Mode().IsRegular() || !os.SameFile(dstInfo, currentDstInfo) {
		return fmt.Errorf("gzip archive destination %q changed during compression", gzPath)
	}
	archiveDir := filepath.Dir(logPath)
	if err := compressArchiveSyncDir(archiveDir); err != nil {
		return fmt.Errorf("persist gzip archive %q: %w", gzPath, err)
	}

	if err := srcFile.Close(); err != nil {
		return fmt.Errorf("close compressed source log %q: %w", logPath, err)
	}
	currentInfo, err := os.Lstat(logPath)
	if err != nil {
		return fmt.Errorf("revalidate compressed source log %q: %w", logPath, err)
	}
	if currentInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(srcInfo, currentInfo) {
		return fmt.Errorf("compressed source log %q changed during compression", logPath)
	}
	if err := compressArchiveRemove(logPath); err != nil {
		return fmt.Errorf("remove compressed source log %q: %w", logPath, err)
	}
	if err := compressArchiveSyncDir(archiveDir); err != nil {
		return fmt.Errorf("persist compressed source log removal %q: %w", logPath, err)
	}

	return nil
}
