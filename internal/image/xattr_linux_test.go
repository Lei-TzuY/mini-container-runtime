//go:build linux

package image

import (
	"archive/tar"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func readXattr(t *testing.T, path, name string) string {
	t.Helper()
	n, err := unix.Getxattr(path, name, nil)
	if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EOPNOTSUPP) {
		t.Skipf("filesystem does not support xattrs: %v", err)
	}
	if err != nil {
		t.Fatalf("getxattr %s %s: %v", path, name, err)
	}
	buf := make([]byte, n)
	n, err = unix.Getxattr(path, name, buf)
	if err != nil {
		t.Fatalf("read xattr %s %s: %v", path, name, err)
	}
	return string(buf[:n])
}

func TestUnpackRestoresRegularAndDirectoryXattrs(t *testing.T) {
	work := t.TempDir()
	tarPath := filepath.Join(work, "layer.tar")
	f, err := os.Create(tarPath)
	if err != nil { t.Fatal(err) }
	tw := tar.NewWriter(f)
	entries := []struct {
		hdr  *tar.Header
		body string
	}{
		{hdr: &tar.Header{Name: "etc/", Typeflag: tar.TypeDir, Mode: 0755, Uid: os.Getuid(), Gid: os.Getgid(), PAXRecords: map[string]string{"SCHILY.xattr.user.minicontainer.dir": "dir-value"}}},
		{hdr: &tar.Header{Name: "etc/config", Typeflag: tar.TypeReg, Mode: 0644, Size: 7, Uid: os.Getuid(), Gid: os.Getgid(), PAXRecords: map[string]string{"SCHILY.xattr.user.minicontainer.file": "payload-xattr"}}, body: "payload"},
	}
	for _, entry := range entries {
		if err := tw.WriteHeader(entry.hdr); err != nil { t.Fatal(err) }
		if entry.body != "" {
			if _, err := tw.Write([]byte(entry.body)); err != nil { t.Fatal(err) }
		}
	}
	if err := tw.Close(); err != nil { t.Fatal(err) }
	if err := f.Close(); err != nil { t.Fatal(err) }

	dest := filepath.Join(work, "rootfs")
	if err := Unpack(tarPath, dest); err != nil { t.Fatalf("unpack: %v", err) }
	if got := readXattr(t, filepath.Join(dest, "etc"), "user.minicontainer.dir"); got != "dir-value" {
		t.Fatalf("directory xattr=%q, want dir-value", got)
	}
	if got := readXattr(t, filepath.Join(dest, "etc", "config"), "user.minicontainer.file"); got != "payload-xattr" {
		t.Fatalf("regular xattr=%q, want payload-xattr", got)
	}
}

func TestTarXattrsPortableIgnoresUnrelatedPAXRecordsAndCopiesValues(t *testing.T) {
	hdr := &tar.Header{PAXRecords: map[string]string{
		"SCHILY.xattr.user.keep": "value",
		"comment":                  "ignore",
		"SCHILY.xattr.":            "ignore-empty-name",
	}}
	xattrs := tarXattrsPortable(hdr)
	if len(xattrs) != 1 || string(xattrs["user.keep"]) != "value" {
		t.Fatalf("parsed xattrs=%v", xattrs)
	}
	hdr.PAXRecords["SCHILY.xattr.user.keep"] = "changed"
	if string(xattrs["user.keep"]) != "value" {
		t.Fatal("parsed xattr aliased mutable header state")
	}
}
