// internal/image/export.go
//
// Container & Image Export (`minictl export` / `minictl commit`)
// ─────────────────────────────────────────────────────────────
// `docker export` creates a tar stream of a container's filesystem.
// `docker commit` creates a new image from a container's current changes.
//
// This module provides functions to package any directory tree (such as a
// container's rootfs or overlay upper layer) into a `.tar` or `.tar.gz` file,
// preserving permissions, file modes, and symlinks.

package image

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ExportDir packs rootDir into tarPath (.tar or .tar.gz).
func ExportDir(rootDir, tarPath string) error {
	rootDir = filepath.Clean(rootDir)
	if _, err := os.Stat(rootDir); os.IsNotExist(err) {
		return fmt.Errorf("source directory %q does not exist", rootDir)
	}

	out, err := os.Create(tarPath)
	if err != nil {
		return fmt.Errorf("create archive %q: %w", tarPath, err)
	}
	defer out.Close()

	var writer io.Writer = out
	var gz *gzip.Writer

	if strings.HasSuffix(strings.ToLower(tarPath), ".gz") || strings.HasSuffix(strings.ToLower(tarPath), ".tgz") {
		gz = gzip.NewWriter(out)
		defer gz.Close()
		writer = gz
	}

	tw := tar.NewWriter(writer)
	defer tw.Close()

	return filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(rootDir, path)
		if err != nil {
			return err
		}

		if rel == "." {
			return nil
		}

		// Convert Windows backslashes to Unix slashes for tar compatibility
		tarName := filepath.ToSlash(rel)
		if info.IsDir() {
			tarName += "/"
		}

		var linkTarget string
		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, err = os.Readlink(path)
			if err != nil {
				return fmt.Errorf("readlink %s: %w", path, err)
			}
		}

		hdr, err := tar.FileInfoHeader(info, linkTarget)
		if err != nil {
			return fmt.Errorf("header %s: %w", path, err)
		}

		hdr.Name = tarName

		if err := tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("write header %s: %w", tarName, err)
		}

		if !info.Mode().IsRegular() {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open %s: %w", path, err)
		}
		defer f.Close()

		if _, err := io.Copy(tw, f); err != nil {
			return fmt.Errorf("copy %s: %w", path, err)
		}

		return nil
	})
}
