package registry

import (
	"archive/tar"
	"os"
	"path/filepath"
	"testing"
)

type registryLayerEntry struct {
	name string
	body string
}

func writeRegistryLayerTar(t *testing.T, path string, entries []registryLayerEntry) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(f)
	for _, entry := range entries {
		body := []byte(entry.body)
		hdr := &tar.Header{
			Name:     entry.name,
			Mode:     0o644,
			Size:     int64(len(body)),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			_ = tw.Close()
			_ = f.Close()
			t.Fatal(err)
		}
		if len(body) != 0 {
			if _, err := tw.Write(body); err != nil {
				_ = tw.Close()
				_ = f.Close()
				t.Fatal(err)
			}
	}
	if err := tw.Close(); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyVerifiedLayersHonorsDeletionWhiteout(t *testing.T) {
	base := t.TempDir()
	lower := filepath.Join(base, "lower.tar")
	upper := filepath.Join(base, "upper.tar")
	writeRegistryLayerTar(t, lower, []registryLayerEntry{
		{name: "keep.txt", body: "keep\n"},
		{name: "deleted.txt", body: "remove me\n"},
	})
	writeRegistryLayerTar(t, upper, []registryLayerEntry{
		{name: ".wh.deleted.txt"},
		{name: "new.txt", body: "new\n"},
	})

	dest := filepath.Join(base, "rootfs")
	if err := applyVerifiedLayers([]string{lower, upper}, dest); err != nil {
		t.Fatalf("applyVerifiedLayers: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "deleted.txt")); !os.IsNotExist(err) {
		t.Fatalf("lower-layer deleted.txt survived whiteout: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, ".wh.deleted.txt")); !os.IsNotExist(err) {
		t.Fatalf("whiteout marker leaked into rootfs: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(dest, "keep.txt")); err != nil || string(data) != "keep\n" {
		t.Fatalf("unrelated lower file changed: data=%q err=%v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(dest, "new.txt")); err != nil || string(data) != "new\n" {
		t.Fatalf("upper file missing: data=%q err=%v", data, err)
	}
}

func TestApplyVerifiedLayersHonorsOpaqueWhiteout(t *testing.T) {
	base := t.TempDir()
	lower := filepath.Join(base, "lower.tar")
	upper := filepath.Join(base, "upper.tar")
	writeRegistryLayerTar(t, lower, []registryLayerEntry{
		{name: "dir/old.txt", body: "old\n"},
		{name: "outside.txt", body: "outside\n"},
	})
	writeRegistryLayerTar(t, upper, []registryLayerEntry{
		{name: "dir/.wh..wh..opq"},
		{name: "dir/new.txt", body: "new\n"},
	})

	dest := filepath.Join(base, "rootfs")
	if err := applyVerifiedLayers([]string{lower, upper}, dest); err != nil {
		t.Fatalf("applyVerifiedLayers: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "dir", "old.txt")); !os.IsNotExist(err) {
		t.Fatalf("opaque whiteout failed to clear lower child: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "dir", ".wh..wh..opq")); !os.IsNotExist(err) {
		t.Fatalf("opaque marker leaked into rootfs: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(dest, "dir", "new.txt")); err != nil || string(data) != "new\n" {
		t.Fatalf("upper opaque-directory file missing: data=%q err=%v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(dest, "outside.txt")); err != nil || string(data) != "outside\n" {
		t.Fatalf("opaque whiteout escaped its directory: data=%q err=%v", data, err)
	}
}
