package builder

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildFromBaseSymlinkDoesNotDereferenceHostTarget(t *testing.T) {
	base := t.TempDir()
	contextDir := filepath.Join(base, "context")
	if err := os.Mkdir(contextDir, 0o700); err != nil {
		t.Fatal(err)
	}
	baseRoot := filepath.Join(base, "base-root")
	if err := os.Mkdir(baseRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(base, "outside.txt")
	if err := os.WriteFile(outside, []byte("sentinel"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(baseRoot, "leak")); err != nil {
		t.Fatal(err)
	}
	outputDir := filepath.Join(base, "output")

	dockerfile := "FROM " + baseRoot + "\nRUN echo contained > /leak\n"
	if err := buildSecurityDockerfile(t, contextDir, outputDir, dockerfile); err != nil {
		t.Fatalf("build from symlinked base tree: %v", err)
	}

	data, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "sentinel" {
		t.Fatalf("FROM/RUN dereferenced host symlink target: %q", data)
	}

	logicalTarget := filepath.Join(outputDir, strings.TrimPrefix(filepath.ToSlash(outside), "/"))
	inside, err := os.ReadFile(logicalTarget)
	if err != nil {
		t.Fatalf("container-relative symlink target missing: %v", err)
	}
	if string(inside) != "contained\n" {
		t.Fatalf("container-relative symlink target=%q", inside)
	}
}
