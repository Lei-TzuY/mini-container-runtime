package diff

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

type ChangeType string

const (
	Added   ChangeType = "A"
	Changed ChangeType = "C"
	Deleted ChangeType = "D"
)

type Change struct {
	Type ChangeType `json:"type"`
	Path string     `json:"path"`
}

// DiffUpper inspects an OverlayFS upper directory and categorizes changes.
func DiffUpper(upperDir string) ([]Change, error) {
	var changes []Change

	err := filepath.WalkDir(upperDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || path == upperDir {
			return nil
		}

		rel, err := filepath.Rel(upperDir, path)
		if err != nil {
			return nil
		}

		relPath := "/" + filepath.ToSlash(rel)
		baseName := d.Name()

		// OverlayFS whiteout character file (deleted file marker `.wh.<filename>`)
		if strings.HasPrefix(baseName, ".wh.") {
			deletedName := strings.TrimPrefix(baseName, ".wh.")
			delRel := filepath.Join(filepath.Dir(rel), deletedName)
			changes = append(changes, Change{
				Type: Deleted,
				Path: "/" + filepath.ToSlash(delRel),
			})
			return nil
		}

		changes = append(changes, Change{
			Type: Added,
			Path: relPath,
		})
		return nil
	})

	return changes, err
}

// DiffDirectories compares targetDir against baseDir file by file.
func DiffDirectories(baseDir, targetDir string) ([]Change, error) {
	var changes []Change
	targetFiles := make(map[string]int64)

	_ = filepath.WalkDir(targetDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || path == targetDir {
			return nil
		}
		rel, _ := filepath.Rel(targetDir, path)
		relPath := "/" + filepath.ToSlash(rel)
		info, err := d.Info()
		if err == nil {
			targetFiles[relPath] = info.Size()
		}
		return nil
	})

	baseFiles := make(map[string]int64)
	_ = filepath.WalkDir(baseDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || path == baseDir {
			return nil
		}
		rel, _ := filepath.Rel(baseDir, path)
		relPath := "/" + filepath.ToSlash(rel)
		info, err := d.Info()
		if err == nil {
			baseFiles[relPath] = info.Size()
		}
		return nil
	})

	for relPath, sz := range targetFiles {
		baseSz, exists := baseFiles[relPath]
		if !exists {
			changes = append(changes, Change{Type: Added, Path: relPath})
		} else if baseSz != sz {
			changes = append(changes, Change{Type: Changed, Path: relPath})
		}
	}

	for relPath := range baseFiles {
		if _, exists := targetFiles[relPath]; !exists {
			changes = append(changes, Change{Type: Deleted, Path: relPath})
		}
	}

	return changes, nil
}

func FormatDiff(changes []Change) string {
	var sb strings.Builder
	for _, c := range changes {
		sb.WriteString(fmt.Sprintf("%s %s\n", c.Type, c.Path))
	}
	return sb.String()
}
