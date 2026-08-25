package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenSecuresStateDirectories(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	if err := os.MkdirAll(filepath.Join(root, "containers"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "images"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := Open(root); err != nil {
		t.Fatalf("Open: %v", err)
	}
	for _, path := range []string{root, filepath.Join(root, "containers"), filepath.Join(root, "images")} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o700 {
			t.Fatalf("%s mode=%#o, want 0700", path, got)
		}
	}
}

func TestOpenRejectsSymlinkStateDirectory(t *testing.T) {
	base := t.TempDir()
	realDir := filepath.Join(base, "real")
	if err := os.Mkdir(realDir, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "state-link")
	if err := os.Symlink(realDir, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(link); err == nil || !strings.Contains(err.Error(), "real directory") {
		t.Fatalf("Open symlink error=%v, want real-directory rejection", err)
	}
}

func TestStateWritesUsePrivateFilesAndLeaveNoTempArtifacts(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(&Container{ID: "secure-container", Status: StatusStopped}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveImage(&Image{Name: "secure:image", RootFS: "/tmp/rootfs"}); err != nil {
		t.Fatal(err)
	}

	paths := []string{
		filepath.Join(store.ctrDir, "secure-container.json"),
		filepath.Join(store.imgDir, "secure_image.json"),
	}
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("%s mode=%#o, want 0600", path, got)
		}
	}
	for _, dir := range []string{store.ctrDir, store.imgDir} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".tmp-") {
				t.Fatalf("temporary artifact left behind: %s", filepath.Join(dir, entry.Name()))
			}
		}
	}
}

func TestContainerStateSymlinkRejected(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte(`{"id":"victim","status":"stopped"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(store.ctrDir, "victim.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get("victim"); err == nil {
		t.Fatal("Get followed symlinked container state")
	}
	if _, err := store.List(); err == nil {
		t.Fatal("List silently accepted symlinked container state")
	}
}

func TestImageStateSymlinkRejected(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte(`{"name":"victim"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(store.imgDir, "victim.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ListImages(); err == nil {
		t.Fatal("ListImages silently accepted symlinked image state")
	}
}

func TestStateListingsFailClosedOnCorruptJSON(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.ctrDir, "broken.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.List(); err == nil {
		t.Fatal("List silently skipped corrupt container state")
	}
	if err := os.Remove(filepath.Join(store.ctrDir, "broken.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.imgDir, "broken.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ListImages(); err == nil {
		t.Fatal("ListImages silently skipped corrupt image state")
	}
}

func TestImageAPIsRejectMissingIdentity(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveImage(nil); err == nil {
		t.Fatal("SaveImage(nil) succeeded")
	}
	if err := store.SaveImage(&Image{}); err == nil {
		t.Fatal("SaveImage without name or ID succeeded")
	}
	for _, selector := range []string{"", "   "} {
		if _, err := store.GetImage(selector); err == nil {
			t.Fatalf("GetImage(%q) succeeded", selector)
		}
		if _, err := store.DeleteImage(selector); err == nil {
			t.Fatalf("DeleteImage(%q) succeeded", selector)
		}
	}
}
