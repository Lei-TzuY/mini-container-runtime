package imagestore

import (
	"os"
	"path/filepath"
	"testing"

	"minicontainer/internal/image"
	"minicontainer/internal/state"
)

func TestImportRawRootFS(t *testing.T) {
	tmpDir := t.TempDir()
	st, err := state.Open(tmpDir)
	if err != nil {
		t.Fatalf("Open state store error: %v", err)
	}

	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("MkdirAll srcDir error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "test.txt"), []byte("sample rootfs content"), 0644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	tarPath := filepath.Join(tmpDir, "test.tar.gz")
	if err := image.ExportDir(srcDir, tarPath); err != nil {
		t.Fatalf("ExportDir error: %v", err)
	}

	rec, err := ImportRawRootFS(st, tarPath, "imported:latest")
	if err != nil {
		t.Fatalf("ImportRawRootFS error: %v", err)
	}

	if rec.Tag != "imported:latest" {
		t.Fatalf("Image tag = %s, want imported:latest", rec.Tag)
	}
}
