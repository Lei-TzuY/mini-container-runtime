// internal/image/image.go
//
// Container Image Unpacking
// ──────────────────────────
// A container image is, at its core, a layered tar archive.  OCI images
// consist of one or more "layers" (each a tar.gz) stacked via overlayfs.
// For this educational runtime we support a single-layer rootfs: either
// a plain .tar or a .tar.gz archive, as produced by:
//
//   • Alpine Linux minirootfs downloads  (alpine-minirootfs-*.tar.gz)
//   • Docker export:  docker export <cid> > rootfs.tar
//   • docker save + manual layer extraction
//
// Security note
// ─────────────
// Tar archives can contain path components like "../../../etc/passwd" that
// would escape the destination directory (a "zip-slip" / tar-slip attack).
// safePath() rejects any entry whose resolved path falls outside destDir.
//
// Entry types handled
// ───────────────────
//   TypeReg      regular file
//   TypeDir      directory
//   TypeSymlink  symbolic link (linkname is stored in the archive header)
//   TypeLink     hard link     (linkname points to another archive entry)
//   TypeChar     character device node (Linux only, requires root)
//   TypeBlock    block device node     (Linux only, requires root)
//   TypeFifo     named pipe            (Linux only)

package image

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Unpack extracts a tar or tar.gz archive to destDir, creating destDir if
// needed.  It prints a summary line on completion.
func Unpack(tarPath, destDir string) error {
	f, err := os.Open(tarPath)
	if err != nil {
		return fmt.Errorf("open %s: %w", tarPath, err)
	}
	defer f.Close()

	var reader io.Reader = f
	lower := strings.ToLower(tarPath)
	if strings.HasSuffix(lower, ".gz") || strings.HasSuffix(lower, ".tgz") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return fmt.Errorf("gzip: %w", err)
		}
		defer gz.Close()
		reader = gz
	}

	destDir = filepath.Clean(destDir)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", destDir, err)
	}

	tr := tar.NewReader(reader)
	var extracted int
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar entry: %w", err)
		}
		target, err := safePath(destDir, hdr.Name)
		if err != nil {
			return err
		}
		if err := applyTarEntry(target, hdr, tr, destDir); err != nil {
			return err
		}
		extracted++
	}

	fmt.Printf("Extracted %d entries → %s\n", extracted, destDir)
	return nil
}

// applyTarEntry writes a single tar entry to the filesystem under destDir.
// It is shared by Unpack (plain tar) and applyLayer (OCI image layers).
// Device-node and FIFO entries that cannot be created are skipped silently.
func applyTarEntry(target string, hdr *tar.Header, r io.Reader, destDir string) error {
	if err := ensureSafeParentDirs(target, destDir); err != nil {
		return err
	}

	switch hdr.Typeflag {
	case tar.TypeDir:
		if fi, err := os.Lstat(target); err == nil && (fi.Mode()&os.ModeSymlink != 0) {
			if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove existing symlink before mkdir %s: %w", target, err)
			}
		}
		return os.MkdirAll(target, hdr.FileInfo().Mode()|0111)

	case tar.TypeReg, tar.TypeRegA:
		// Linux writes re-traverse from a pinned extraction-root dirfd with
		// O_NOFOLLOW and perform leaf unlink/create relative to the pinned parent.
		// This keeps a concurrent parent rename/symlink replacement from turning
		// the pathname safety check above into an out-of-root write.
		return writeRegularSecure(target, destDir, hdr, r)

	case tar.TypeSymlink:
		return createSymlinkSecure(target, destDir, hdr.Linkname)

	case tar.TypeLink:
		linkTarget, err := safePath(destDir, hdr.Linkname)
		if err != nil {
			return err
		}
		return createHardlinkSecure(target, destDir, linkTarget)

	case tar.TypeChar, tar.TypeBlock, tar.TypeFifo:
		if err := makeSpecialSecure(target, destDir, hdr); err != nil {
			if !strings.Contains(err.Error(), "not supported") {
				fmt.Fprintf(os.Stderr, "warning: mknod %s: %v\n", target, err)
			}
		}
	}
	return nil
}

// ensureSafeParentDirs verifies that all ancestor directory components of target
// within destDir exist and do not point outside destDir via symlinks.
func ensureSafeParentDirs(target, destDir string) error {
	destAbs, err := filepath.Abs(destDir)
	if err != nil {
		return err
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return err
	}

	rel, err := filepath.Rel(destAbs, targetAbs)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path traversal detected: %q escapes %q", target, destDir)
	}

	parts := strings.Split(filepath.Dir(rel), string(filepath.Separator))
	curr := destAbs
	for _, part := range parts {
		if part == "." || part == "" {
			continue
		}
		curr = filepath.Join(curr, part)
		fi, err := os.Lstat(curr)
		if err == nil {
			if fi.Mode()&os.ModeSymlink != 0 {
				eval, err := filepath.EvalSymlinks(curr)
				if err != nil || !isSubDir(destAbs, eval) {
					return fmt.Errorf("symlink path traversal detected: directory component %q escapes destination", curr)
				}
			}
		} else if os.IsNotExist(err) {
			if err := os.Mkdir(curr, 0755); err != nil && !os.IsExist(err) {
				return err
			}
		} else {
			return err
		}
	}
	return nil
}

func isSubDir(base, target string) bool {
	baseAbs, err1 := filepath.Abs(base)
	targetAbs, err2 := filepath.Abs(target)
	if err1 != nil || err2 != nil {
		return false
	}
	rel, err := filepath.Rel(baseAbs, targetAbs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

// safePath joins base and name, and returns an error if the result escapes base.
// This prevents tar-slip (directory traversal) attacks.
func safePath(base, name string) (string, error) {
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return "", fmt.Errorf("resolve destination %q: %w", base, err)
	}

	normalized := strings.ReplaceAll(name, "\\", "/")
	if strings.HasPrefix(normalized, "/") {
		return "", fmt.Errorf("path traversal detected: %q escapes destination", name)
	}
	trimmed := strings.TrimLeft(normalized, "/")
	if hasWindowsDrivePrefix(trimmed) {
		return "", fmt.Errorf("path traversal detected: %q escapes destination", name)
	}
	for _, part := range strings.Split(normalized, "/") {
		if part == ".." {
			return "", fmt.Errorf("path traversal detected: %q escapes destination", name)
		}
	}

	cleaned := strings.TrimPrefix(path.Clean("/"+normalized), "/")
	if cleaned == "." {
		cleaned = ""
	}

	target := filepath.Join(baseAbs, filepath.FromSlash(cleaned))
	rel, err := filepath.Rel(baseAbs, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path traversal detected: %q escapes destination", name)
	}
	return target, nil
}

func hasWindowsDrivePrefix(name string) bool {
	return len(name) >= 2 && name[1] == ':' &&
		((name[0] >= 'a' && name[0] <= 'z') || (name[0] >= 'A' && name[0] <= 'Z'))
}
