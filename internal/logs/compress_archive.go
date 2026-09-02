package logs

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

var (
	compressArchiveRemove    = os.Remove
	compressArchiveGzipClose = func(w *gzip.Writer) error { return w.Close() }
)

// CompressRotatedLog compresses logPath to logPath.gz and removes the uncompressed file.
func CompressRotatedLog(logPath string) error {
	srcFile, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("open log file: %w", err)
	}
	defer srcFile.Close()

	gzPath := logPath + ".gz"
	dstFile, err := os.OpenFile(gzPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|unix.O_NOFOLLOW, 0644)
	if err != nil {
		return fmt.Errorf("create gz file: %w", err)
	}
	defer dstFile.Close()

	gzWriter := gzip.NewWriter(dstFile)

	if _, err := io.Copy(gzWriter, srcFile); err != nil {
		_ = gzWriter.Close()
		return fmt.Errorf("gzip compress: %w", err)
	}
	if err := compressArchiveGzipClose(gzWriter); err != nil {
		return fmt.Errorf("finalize gzip archive %q: %w", gzPath, err)
	}

	if err := srcFile.Close(); err != nil {
		return fmt.Errorf("close compressed source log %q: %w", logPath, err)
	}
	if err := compressArchiveRemove(logPath); err != nil {
		return fmt.Errorf("remove compressed source log %q: %w", logPath, err)
	}

	return nil
}
