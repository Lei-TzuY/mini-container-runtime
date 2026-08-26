package imagestore

import (
	"crypto/sha256"
	"fmt"
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
	defer st.Close()

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
	if _, err := os.Stat(filepath.Join(rec.RootFS, "test.txt")); err != nil {
		t.Fatalf("published rootfs missing content: %v", err)
	}
}

func TestImportRawRootFSFailureDoesNotPublishPartialImage(t *testing.T) {
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	bad := []byte("not a gzip stream")
	tarPath := filepath.Join(t.TempDir(), "broken.tar.gz")
	if err := os.WriteFile(tarPath, bad, 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := ImportRawRootFS(st, tarPath, "broken:latest"); err == nil {
		t.Fatal("malformed archive import unexpectedly succeeded")
	}

	sum := fmt.Sprintf("%x", sha256.Sum256(bad))[:12]
	if _, err := os.Stat(filepath.Join(st.Dir(), "images", sum)); !os.IsNotExist(err) {
		t.Fatalf("failed import published image directory: err=%v", err)
	}
	staging, err := filepath.Glob(filepath.Join(st.Dir(), "images", ".import-"+sum+"-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(staging) != 0 {
		t.Fatalf("failed import left staging directories: %v", staging)
	}
}
