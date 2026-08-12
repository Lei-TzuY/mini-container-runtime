package diff

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiffUpper(t *testing.T) {
	tmpDir := t.TempDir()
	upper := filepath.Join(tmpDir, "upper")
	_ = os.MkdirAll(filepath.Join(upper, "app"), 0755)
	_ = os.MkdirAll(filepath.Join(upper, "etc"), 0755)

	_ = os.WriteFile(filepath.Join(upper, "app", "server.js"), []byte("console.log('hi')"), 0644)
	_ = os.WriteFile(filepath.Join(upper, "etc", ".wh.oldconfig"), []byte(""), 0644)

	changes, err := DiffUpper(upper)
	if err != nil {
		t.Fatalf("DiffUpper error: %v", err)
	}

	formatted := FormatDiff(changes)
	if !strings.Contains(formatted, "A /app/server.js") {
		t.Fatalf("Missing added file in diff:\n%s", formatted)
	}
	if !strings.Contains(formatted, "D /etc/oldconfig") {
		t.Fatalf("Missing deleted whiteout in diff:\n%s", formatted)
	}
}

func TestDiffDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	base := filepath.Join(tmpDir, "base")
	target := filepath.Join(tmpDir, "target")

	_ = os.MkdirAll(base, 0755)
	_ = os.MkdirAll(target, 0755)

	_ = os.WriteFile(filepath.Join(base, "file1.txt"), []byte("v1"), 0644)
	_ = os.WriteFile(filepath.Join(base, "file2.txt"), []byte("deleted"), 0644)

	_ = os.WriteFile(filepath.Join(target, "file1.txt"), []byte("v2-modified"), 0644)
	_ = os.WriteFile(filepath.Join(target, "file3.txt"), []byte("added"), 0644)

	changes, err := DiffDirectories(base, target)
	if err != nil {
		t.Fatalf("DiffDirectories error: %v", err)
	}

	formatted := FormatDiff(changes)
	if !strings.Contains(formatted, "C /file1.txt") {
		t.Fatalf("Missing changed file in diff:\n%s", formatted)
	}
	if !strings.Contains(formatted, "A /file3.txt") {
		t.Fatalf("Missing added file in diff:\n%s", formatted)
	}
	if !strings.Contains(formatted, "D /file2.txt") {
		t.Fatalf("Missing deleted file in diff:\n%s", formatted)
	}
}
