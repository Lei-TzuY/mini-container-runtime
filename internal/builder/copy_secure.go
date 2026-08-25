package builder

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
)

func copyRegularFile(src, dstRoot, dstLogical string, mode os.FileMode) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("source %q is not a regular file", src)
	}
	if err := mkdirRootFSPath(dstRoot, path.Dir(dstLogical), 0o755); err != nil {
		return err
	}
	dst, err := resolveRootFSPath(dstRoot, dstLogical)
	if err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func copySymlink(src, dstRoot, dstLogical string) error {
	target, err := os.Readlink(src)
	if err != nil {
		return err
	}
	if err := mkdirRootFSPath(dstRoot, path.Dir(dstLogical), 0o755); err != nil {
		return err
	}
	dst, err := resolveRootFSLeaf(dstRoot, dstLogical)
	if err != nil {
		return err
	}
	if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("replace destination symlink %q: %w", dstLogical, err)
	}
	if err := os.Symlink(target, dst); err != nil {
		return fmt.Errorf("copy symlink %q: %w", src, err)
	}
	return nil
}

// copyTree copies a source tree into dstLogical. When allowSymlinks is true,
// symlinks inside a directory tree are recreated and never dereferenced on the
// host. The source root itself must be a real directory/file.
func copyTree(src, dstRoot, dstLogical string, allowSymlinks bool) error {
	srcInfo, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if srcInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("build source root %q must not be a symlink", src)
	}
	if !srcInfo.IsDir() {
		return copyRegularFile(src, dstRoot, dstLogical, srcInfo.Mode())
	}

	return filepath.Walk(src, func(sourcePath string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, sourcePath)
		if err != nil {
			return err
		}
		relSlash := filepath.ToSlash(rel)
		targetLogical := dstLogical
		if relSlash != "." {
			targetLogical = path.Join(dstLogical, relSlash)
		}

		if info.Mode()&os.ModeSymlink != 0 {
			if !allowSymlinks {
				return fmt.Errorf("COPY source tree contains symlink %q", sourcePath)
			}
			return copySymlink(sourcePath, dstRoot, targetLogical)
		}
		if info.IsDir() {
			return mkdirRootFSPath(dstRoot, targetLogical, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported special file %q in build source", sourcePath)
		}
		return copyRegularFile(sourcePath, dstRoot, targetLogical, info.Mode())
	})
}

func destinationIsDirectory(root, logical string) (bool, error) {
	hostPath, err := resolveRootFSPath(root, logical)
	if err != nil {
		return false, err
	}
	info, err := os.Stat(hostPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return info.IsDir(), nil
}
