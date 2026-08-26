//go:build linux

package image

import (
	"archive/tar"
	"os"
	"path/filepath"
	"testing"
)

func TestMakeSpecialSecurePinsParentAgainstSymlinkReplacement(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	parent := filepath.Join(root, "parent")
	pinnedParent := filepath.Join(root, "parent-pinned")
	if err := os.Mkdir(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(parent, "pipe")
	hdr := &tar.Header{Typeflag: tar.TypeFifo, Mode: 0o600}

	err := makeSpecialSecureWithHook(target, root, hdr, func() {
		if err := os.Rename(parent, pinnedParent); err != nil {
			t.Fatalf("rename parent: %v", err)
		}
		if err := os.Symlink(outside, parent); err != nil {
			t.Fatalf("replace parent with outside symlink: %v", err)
		}
	})
	if err != nil {
		t.Fatalf("secure FIFO creation: %v", err)
	}

	fi, err := os.Lstat(filepath.Join(pinnedParent, "pipe"))
	if err != nil {
		t.Fatalf("pinned FIFO missing: %v", err)
	}
	if fi.Mode()&os.ModeNamedPipe == 0 {
		t.Fatalf("pinned target mode=%v, want named pipe", fi.Mode())
	}
	if _, err := os.Lstat(filepath.Join(outside, "pipe")); !os.IsNotExist(err) {
		t.Fatalf("outside target was touched: %v", err)
	}
}

func TestMakeSpecialSecureRefusesDirectoryReplacement(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "keep")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	hdr := &tar.Header{Typeflag: tar.TypeFifo, Mode: 0o600}
	if err := makeSpecialSecure(target, root, hdr); err == nil {
		t.Fatal("directory replacement unexpectedly succeeded")
	}
	fi, err := os.Lstat(target)
	if err != nil {
		t.Fatalf("preserved directory missing: %v", err)
	}
	if !fi.IsDir() {
		t.Fatalf("target mode=%v, want preserved directory", fi.Mode())
	}
}
