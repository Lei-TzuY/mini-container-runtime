//go:build linux

package image

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateHardlinkSecurePinsSourceAndDestinationParents(t *testing.T) {
	root := t.TempDir()
	sourceParent := filepath.Join(root, "src")
	destParent := filepath.Join(root, "dst")
	if err := os.Mkdir(sourceParent, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(destParent, 0755); err != nil {
		t.Fatal(err)
	}

	source := filepath.Join(sourceParent, "payload")
	if err := os.WriteFile(source, []byte("owned-generation"), 0644); err != nil {
		t.Fatal(err)
	}

	outsideSource := t.TempDir()
	outsideDest := t.TempDir()
	if err := os.WriteFile(filepath.Join(outsideSource, "payload"), []byte("foreign-generation"), 0644); err != nil {
		t.Fatal(err)
	}

	pinnedSourceParent := filepath.Join(root, "src-pinned")
	pinnedDestParent := filepath.Join(root, "dst-pinned")
	target := filepath.Join(destParent, "copy")

	err := createHardlinkSecureWithHook(target, root, source, func() {
		if err := os.Rename(sourceParent, pinnedSourceParent); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outsideSource, sourceParent); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(destParent, pinnedDestParent); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outsideDest, destParent); err != nil {
			t.Fatal(err)
		}
	})
	if err != nil {
		t.Fatalf("secure hardlink after parent replacement: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(pinnedDestParent, "copy"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "owned-generation" {
		t.Fatalf("hardlink content = %q, want pinned source content", got)
	}
	if _, err := os.Lstat(filepath.Join(outsideDest, "copy")); !os.IsNotExist(err) {
		t.Fatalf("outside destination was modified: err=%v", err)
	}

	sourceInfo, err := os.Stat(filepath.Join(pinnedSourceParent, "payload"))
	if err != nil {
		t.Fatal(err)
	}
	linkInfo, err := os.Stat(filepath.Join(pinnedDestParent, "copy"))
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(sourceInfo, linkInfo) {
		t.Fatal("destination is not a hardlink to the pinned source inode")
	}
}

func TestCreateHardlinkSecureRefusesDirectoryDestination(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "source"), []byte("payload"), 0644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "existing-dir")
	if err := os.Mkdir(target, 0755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(target, "keep")
	if err := os.WriteFile(marker, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := createHardlinkSecure(target, root, filepath.Join(root, "source")); err == nil {
		t.Fatal("directory hardlink destination was accepted")
	}
	if got, err := os.ReadFile(marker); err != nil || string(got) != "keep" {
		t.Fatalf("directory destination was destructively modified: content=%q err=%v", got, err)
	}
}

func TestCreateHardlinkSecureRejectsSymlinkParent(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "source"), []byte("payload"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "dst")); err != nil {
		t.Fatal(err)
	}

	err := createHardlinkSecure(filepath.Join(root, "dst", "copy"), root, filepath.Join(root, "source"))
	if err == nil {
		t.Fatal("symlink destination parent was accepted")
	}
	if _, err := os.Lstat(filepath.Join(outside, "copy")); !os.IsNotExist(err) {
		t.Fatalf("outside directory was modified: err=%v", err)
	}
}
