package image

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestUnpackTarGz(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "rootfs.tar.gz")
	dest := filepath.Join(t.TempDir(), "rootfs")

	writeTarGz(t, archive, []tarEntry{
		{header: &tar.Header{Name: "bin/", Typeflag: tar.TypeDir, Mode: 0755}},
		{header: &tar.Header{Name: "bin/hello", Typeflag: tar.TypeReg, Mode: 0755, Size: int64(len("hello\n"))}, body: "hello\n"},
		{header: &tar.Header{Name: "etc/os-release", Typeflag: tar.TypeReg, Mode: 0644, Size: int64(len("NAME=minicontainer\n"))}, body: "NAME=minicontainer\n"},
		{header: &tar.Header{Name: "bin/hello-hardlink", Typeflag: tar.TypeLink, Linkname: "bin/hello", Mode: 0755}},
	})

	if err := Unpack(archive, dest); err != nil {
		t.Fatalf("Unpack returned error: %v", err)
	}

	assertFile(t, filepath.Join(dest, "bin", "hello"), "hello\n")
	assertFile(t, filepath.Join(dest, "etc", "os-release"), "NAME=minicontainer\n")
	assertFile(t, filepath.Join(dest, "bin", "hello-hardlink"), "hello\n")
}

func TestUnpackRejectsPathTraversal(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "bad.tar")
	destParent := t.TempDir()
	dest := filepath.Join(destParent, "rootfs")

	writeTar(t, archive, []tarEntry{
		{header: &tar.Header{Name: "../escape", Typeflag: tar.TypeReg, Mode: 0644, Size: int64(len("bad"))}, body: "bad"},
	})

	if err := Unpack(archive, dest); err == nil {
		t.Fatalf("Unpack succeeded, want traversal error")
	}
	if _, err := os.Stat(filepath.Join(destParent, "escape")); !os.IsNotExist(err) {
		t.Fatalf("escape file exists or stat failed unexpectedly: %v", err)
	}
}

func TestUnpackSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks on Windows may require elevated privileges")
	}

	archive := filepath.Join(t.TempDir(), "rootfs.tar")
	dest := filepath.Join(t.TempDir(), "rootfs")

	writeTar(t, archive, []tarEntry{
		{header: &tar.Header{Name: "bin/target", Typeflag: tar.TypeReg, Mode: 0644, Size: int64(len("target"))}, body: "target"},
		{header: &tar.Header{Name: "bin/link", Typeflag: tar.TypeSymlink, Linkname: "target", Mode: 0777}},
	})

	if err := Unpack(archive, dest); err != nil {
		t.Fatalf("Unpack returned error: %v", err)
	}

	target, err := os.Readlink(filepath.Join(dest, "bin", "link"))
	if err != nil {
		t.Fatalf("Readlink returned error: %v", err)
	}
	if target != "target" {
		t.Fatalf("symlink target = %q, want target", target)
	}
}

func TestSafePath(t *testing.T) {
	base := filepath.Join(t.TempDir(), "rootfs")

	target, err := safePath(base, "usr/bin/sh")
	if err != nil {
		t.Fatalf("safePath returned error: %v", err)
	}
	if target != filepath.Join(base, "usr", "bin", "sh") {
		t.Fatalf("safePath target = %q", target)
	}

	for _, name := range []string{"../escape", "/absolute", "/../escape", "usr/../../escape", `C:\Windows\system32`} {
		t.Run(name, func(t *testing.T) {
			if _, err := safePath(base, name); err == nil {
				t.Fatalf("safePath(%q) succeeded, want error", name)
			}
		})
	}
}

type tarEntry struct {
	header *tar.Header
	body   string
}

func writeTarGz(t *testing.T, path string, entries []tarEntry) {
	t.Helper()

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	defer f.Close()

	gz := gzip.NewWriter(f)
	defer gz.Close()

	tw := tar.NewWriter(gz)
	defer tw.Close()

	writeEntries(t, tw, entries)
}

func writeTar(t *testing.T, path string, entries []tarEntry) {
	t.Helper()

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	defer f.Close()

	tw := tar.NewWriter(f)
	defer tw.Close()

	writeEntries(t, tw, entries)
}

func writeEntries(t *testing.T, tw *tar.Writer, entries []tarEntry) {
	t.Helper()

	for _, entry := range entries {
		if err := tw.WriteHeader(entry.header); err != nil {
			t.Fatalf("write header %q: %v", entry.header.Name, err)
		}
		if entry.body == "" {
			continue
		}
		if _, err := tw.Write([]byte(entry.body)); err != nil {
			t.Fatalf("write body %q: %v", entry.header.Name, err)
		}
	}
}

func assertFile(t *testing.T, path string, want string) {
	t.Helper()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q, want %q", path, string(got), want)
	}
}
