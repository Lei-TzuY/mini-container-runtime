package logs

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
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
	dstFile, err := os.Create(gzPath)
	if err != nil {
		return fmt.Errorf("create gz file: %w", err)
	}
	defer dstFile.Close()

	gzWriter := gzip.NewWriter(dstFile)
	defer gzWriter.Close()

	if _, err := io.Copy(gzWriter, srcFile); err != nil {
		return fmt.Errorf("gzip compress: %w", err)
	}

	_ = gzWriter.Close()
	_ = srcFile.Close()
	_ = os.Remove(logPath)

	return nil
}
