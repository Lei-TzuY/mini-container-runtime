// internal/image/cp.go
//
// Container File Transfer (`minictl cp`)
// ─────────────────────────────────────────
// Copies files and directories bidirectionally between host filesystem and
// container RootFS (`minictl cp hostPath id:containerPath` / `minictl cp id:containerPath hostPath`).

package image

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// CopyBetweenHostAndContainer performs file/directory copy between host and container rootfs.
func CopyPath(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat source %q: %w", src, err)
	}

	if info.IsDir() {
		return copyDir(src, dst)
	}
	return copyFile(src, dst)
}

func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	if dstInfo, err := os.Stat(dst); err == nil && dstInfo.IsDir() {
		dst = filepath.Join(dst, filepath.Base(src))
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}
	return nil
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFile(path, target)
	})
}

// ParseCopyTarget splits "id:/path" into ("id", "/path") or returns ("", arg) if plain path.
func ParseCopyTarget(arg string) (string, string) {
	if idx := strings.Index(arg, ":"); idx != -1 && !strings.Contains(arg[:idx], "/") && !strings.Contains(arg[:idx], "\\") {
		return arg[:idx], arg[idx+1:]
	}
	return "", arg
}
