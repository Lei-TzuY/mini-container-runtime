// internal/image/oci.go
//
// Docker-save Image Loading
// ──────────────────────────
// "docker save <image> -o image.tar" produces a tar archive with the
// following layout:
//
//   image.tar
//   ├── manifest.json          ← image list (Config, RepoTags, Layers)
//   ├── <image-id>.json        ← image config (env, entrypoint, …)
//   └── <layer-hash>/
//       └── layer.tar          ← one filesystem diff per layer
//
// manifest.json schema (array — one entry per image):
//
//   [{ "Config": "<id>.json",
//      "RepoTags": ["alpine:3.19"],
//      "Layers": ["<hash>/layer.tar", …] }]
//
// Layer stacking
// ──────────────
// Layers are applied in order; later entries overwrite earlier ones.
// Deletions are encoded as OCI whiteout files inside the layer tar:
//
//   .wh.<name>        marks <name> in the same directory as deleted
//   .wh..wh..opq      opaque whiteout — the whole directory is replaced
//                     (all files from earlier layers in this dir are removed)
//
// These conventions originate from the overlay filesystem semantics used by
// the container image spec: https://github.com/opencontainers/image-spec

package image

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	whiteoutPrefix = ".wh."
	whiteoutOpaque = ".wh..wh..opq"
)

// dockerManifest is one entry in the manifest.json written by "docker save".
type dockerManifest struct {
	Config   string   `json:"Config"`
	RepoTags []string `json:"RepoTags"`
	Layers   []string `json:"Layers"`
}

// LoadDockerSave reads a docker-save archive (produced by "docker save") and
// extracts all image layers into destDir in order, applying whiteout semantics.
//
// The archive may be a plain .tar or a .tar.gz.
func LoadDockerSave(tarPath, destDir string) error {
	// Extract the outer docker-save tar into a temp directory.
	// Unpack handles both plain and gzipped archives.
	tmpDir, err := os.MkdirTemp("", "minicontainer-load-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := Unpack(tarPath, tmpDir); err != nil {
		return fmt.Errorf("extract save archive: %w", err)
	}

	raw, err := os.ReadFile(filepath.Join(tmpDir, "manifest.json"))
	if err != nil {
		return fmt.Errorf("read manifest.json: %w", err)
	}

	var manifests []dockerManifest
	if err := json.Unmarshal(raw, &manifests); err != nil {
		return fmt.Errorf("parse manifest.json: %w", err)
	}
	if len(manifests) == 0 {
		return fmt.Errorf("manifest.json contains no images")
	}
	if len(manifests) > 1 {
		fmt.Fprintf(os.Stderr, "warning: save archive has %d images; loading the first one\n", len(manifests))
	}

	m := manifests[0]
	tag := "<untagged>"
	if len(m.RepoTags) > 0 {
		tag = m.RepoTags[0]
	}
	fmt.Printf("Loading %s  (%d layer(s))\n", tag, len(m.Layers))

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", destDir, err)
	}

	for i, rel := range m.Layers {
		// Layer paths in manifest.json must not escape tmpDir.
		layerPath, err := safePath(tmpDir, rel)
		if err != nil {
			return fmt.Errorf("layer %d has invalid path %q: %w", i+1, rel, err)
		}
		fmt.Printf("  [%d/%d] applying layer\n", i+1, len(m.Layers))
		if err := applyLayer(layerPath, destDir); err != nil {
			return fmt.Errorf("layer %d: %w", i+1, err)
		}
	}

	fmt.Printf("Done. Rootfs ready at %s\n", destDir)
	return nil
}

// applyLayer extracts one image layer tar onto destDir, handling whiteouts.
func applyLayer(layerPath, destDir string) error {
	rc, err := openMaybeGzip(layerPath)
	if err != nil {
		return err
	}
	defer rc.Close()

	tr := tar.NewReader(rc)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read layer entry: %w", err)
		}

		base := filepath.Base(hdr.Name)
		dir := filepath.Dir(hdr.Name)

		// ── Opaque whiteout ────────────────────────────────────────────────
		// .wh..wh..opq: this directory is fully replaced by this layer.
		// Remove everything the earlier layers put here before applying new
		// contents.
		if base == whiteoutOpaque {
			targetDir, err := safePath(destDir, dir)
			if err != nil {
				return fmt.Errorf("opaque whiteout invalid path %q: %w", dir, err)
			}
			if err := ensureSafeParentDirs(targetDir, destDir); err != nil {
				return fmt.Errorf("opaque whiteout unsafe parent %q: %w", targetDir, err)
			}
			if entries, err := os.ReadDir(targetDir); err == nil {
				for _, e := range entries {
					_ = os.RemoveAll(filepath.Join(targetDir, e.Name()))
				}
			}
			continue
		}

		// ── Regular whiteout ───────────────────────────────────────────────
		// .wh.<name>: delete one specific file or directory.
		if strings.HasPrefix(base, whiteoutPrefix) {
			deleted := strings.TrimPrefix(base, whiteoutPrefix)
			target, err := safePath(destDir, filepath.Join(dir, deleted))
			if err != nil {
				return fmt.Errorf("whiteout invalid path %q: %w", filepath.Join(dir, deleted), err)
			}
			if err := ensureSafeParentDirs(target, destDir); err != nil {
				return fmt.Errorf("whiteout unsafe parent %q: %w", target, err)
			}
			_ = os.RemoveAll(target)
			continue
		}

		// ── Normal entry ───────────────────────────────────────────────────
		target, err := safePath(destDir, hdr.Name)
		if err != nil {
			return err
		}
		if err := applyTarEntry(target, hdr, tr, destDir); err != nil {
			return err
		}
	}
	return nil
}

// openMaybeGzip opens path and returns a ReadCloser.  If the first two bytes
// are the gzip magic number (0x1f 0x8b) it wraps the file in a gzip reader;
// otherwise it returns the raw file.  The caller must Close the result.
func openMaybeGzip(path string) (io.ReadCloser, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	// Peek at the first two bytes without consuming them.
	br := bufio.NewReaderSize(f, 512)
	magic, err := br.Peek(2)
	if err != nil {
		// File is too short or empty — return as-is (tar will handle it).
		return &plainFile{Reader: br, f: f}, nil
	}

	if magic[0] == 0x1f && magic[1] == 0x8b {
		gz, err := gzip.NewReader(br)
		if err != nil {
			f.Close()
			return nil, fmt.Errorf("gzip open %s: %w", path, err)
		}
		return &gzipFile{Reader: gz, f: f}, nil
	}

	return &plainFile{Reader: br, f: f}, nil
}

// plainFile wraps a bufio.Reader (with peeked bytes) + the underlying *os.File.
type plainFile struct {
	io.Reader // bufio.Reader — preserves peeked bytes
	f         *os.File
}

func (p *plainFile) Close() error { return p.f.Close() }

// gzipFile wraps a gzip.Reader over a bufio.Reader over an *os.File,
// closing both the gzip stream and the underlying file on Close.
type gzipFile struct {
	*gzip.Reader
	f *os.File
}

func (g *gzipFile) Close() error {
	err1 := g.Reader.Close()
	err2 := g.f.Close()
	if err1 != nil {
		return err1
	}
	return err2
}
