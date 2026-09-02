package logs

import (
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCompressRotatedLog(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "container.log.1")
	_ = os.WriteFile(logPath, []byte("log data content"), 0644)

	if err := CompressRotatedLog(logPath); err != nil {
		t.Fatalf("CompressRotatedLog error: %v", err)
	}

	if _, err := os.Stat(logPath + ".gz"); err != nil {
		t.Fatalf("Compressed archive container.log.1.gz does not exist: %v", err)
	}
}

func TestCompressRotatedLogReportsSourceRemovalFailure(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "container.log.1")
	if err := os.WriteFile(logPath, []byte("log data content"), 0644); err != nil {
		t.Fatal(err)
	}

	oldRemove := compressArchiveRemove
	wantErr := errors.New("remove failed")
	compressArchiveRemove = func(string) error { return wantErr }
	defer func() { compressArchiveRemove = oldRemove }()

	err := CompressRotatedLog(logPath)
	if err == nil {
		t.Fatal("expected source removal failure to be reported")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("CompressRotatedLog error = %v, want wrapped removal failure", err)
	}
	if _, statErr := os.Stat(logPath); statErr != nil {
		t.Fatalf("source log should remain after failed removal: %v", statErr)
	}
}

func TestCompressRotatedLogReportsGzipFinalizeFailure(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "container.log.1")
	if err := os.WriteFile(logPath, []byte("log data content"), 0644); err != nil {
		t.Fatal(err)
	}

	oldClose := compressArchiveGzipClose
	wantErr := errors.New("gzip finalize failed")
	compressArchiveGzipClose = func(*gzip.Writer) error { return wantErr }
	defer func() { compressArchiveGzipClose = oldClose }()

	err := CompressRotatedLog(logPath)
	if err == nil {
		t.Fatal("expected gzip finalize failure to be reported")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("CompressRotatedLog error = %v, want wrapped finalize failure", err)
	}
	if _, statErr := os.Stat(logPath); statErr != nil {
		t.Fatalf("source log should remain after failed gzip finalize: %v", statErr)
	}
}
