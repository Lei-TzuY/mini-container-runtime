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
	"errors"
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
// The archive may be a plain .tar or a .tar.gz. If destDir does not exist when
// loading begins, layers are assembled in a private sibling directory and the
// completed rootfs is published without replacing a concurrently-created path.
// Existing destinations retain the historical in-place overlay behavior.
func LoadDockerSave(tarPath, destDir string) (retErr error) {
	tmpDir, err := os.MkdirTemp("", "minicontainer-load-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := Unpack(tarPath, tmpDir); err != nil {
		return fmt.Errorf("extract save archive: %w", err)
	}

	manifestFile, err := openDockerSaveMember(tmpDir, "manifest.json")
	if err != nil {
		return fmt.Errorf("open manifest.json: %w", err)
	}
	raw, readErr := io.ReadAll(manifestFile)
	closeErr := manifestFile.Close()
	if readErr != nil {
		return fmt.Errorf("read manifest.json: %w", readErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close manifest.json: %w", closeErr)
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

	// Prove and pin every archive-selected layer before mutating the destination.
	// A bad later member must not be discovered only after earlier layers have
	// already changed an existing rootfs.
	layerFiles := make([]*os.File, len(m.Layers))
	defer func() {
		for i, f := range layerFiles {
			if f == nil {
				continue
			}
			if err := f.Close(); err != nil {
				retErr = errors.Join(retErr, fmt.Errorf("close pinned docker-save layer %d: %w", i+1, err))
			}
		}
	}()
	for i, rel := range m.Layers {
		layerFile, err := openDockerSaveMember(tmpDir, rel)
		if err != nil {
			return fmt.Errorf("layer %d has invalid member %q: %w", i+1, rel, err)
		}
		layerFiles[i] = layerFile
	}

	destDir = filepath.Clean(destDir)
	workDest := destDir
	stagingDir := ""
	if _, err := os.Lstat(destDir); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("inspect load destination %s: %w", destDir, err)
		}
		parent := filepath.Dir(destDir)
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return fmt.Errorf("create load destination parent %s: %w", parent, err)
		}
		stagingDir, err = os.MkdirTemp(parent, "."+filepath.Base(destDir)+".load-*")
		if err != nil {
			return fmt.Errorf("create load destination staging directory: %w", err)
		}
		workDest = stagingDir
		defer func() {
			if stagingDir == "" {
				return
			}
			if err := os.RemoveAll(stagingDir); err != nil {
				retErr = errors.Join(retErr, fmt.Errorf("remove failed load staging directory %q: %w", stagingDir, err))
			}
		}()
	} else if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", destDir, err)
	}

	for i, rel := range m.Layers {
		layerFile := layerFiles[i]
		layerFiles[i] = nil // applyOpenedLayer assumes ownership and closes it.
		fmt.Printf("  [%d/%d] applying layer\n", i+1, len(m.Layers))
		if err := applyOpenedLayer(layerFile, rel, workDest); err != nil {
			return fmt.Errorf("layer %d: %w", i+1, err)
		}
	}

	if stagingDir != "" {
		if err := publishDirectoryNoReplace(stagingDir, destDir); err != nil {
			return err
		}
		stagingDir = ""
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
	return applyLayerReader(rc, destDir)
}

func applyOpenedLayer(layerFile *os.File, label, destDir string) error {
	rc, err := openMaybeGzipFile(layerFile, label)
	if err != nil {
		return err
	}
	defer rc.Close()
	return applyLayerReader(rc, destDir)
}

func applyLayerReader(r io.Reader, destDir string) error {
	tr := tar.NewReader(r)
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

		// .wh..wh..opq removes all lower-layer children while preserving the
		// directory itself. Linux uses pinned dirfds so a parent replacement
		// cannot redirect recursive deletion outside the extraction root.
		if base == whiteoutOpaque {
			targetDir, err := safePath(destDir, dir)
			if err != nil {
				return fmt.Errorf("opaque whiteout invalid path %q: %w", dir, err)
			}
			if err := clearOpaqueWhiteoutSecure(targetDir, destDir); err != nil {
				return fmt.Errorf("opaque whiteout cleanup %q: %w", targetDir, err)
			}
			continue
		}

		// .wh.<name> recursively removes the lower-layer path. Linux resolves
		// and removes it relative to a pinned extraction-root generation.
		if strings.HasPrefix(base, whiteoutPrefix) {
			deleted := strings.TrimPrefix(base, whiteoutPrefix)
			target, err := safePath(destDir, filepath.Join(dir, deleted))
			if err != nil {
				return fmt.Errorf("whiteout invalid path %q: %w", filepath.Join(dir, deleted), err)
			}
			if err := removeWhiteoutSecure(target, destDir); err != nil {
				return fmt.Errorf("whiteout cleanup %q: %w", target, err)
			}
			continue
		}

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

// openMaybeGzip opens path and returns a ReadCloser. If the first two bytes are
// the gzip magic number it wraps the file in a gzip reader.
func openMaybeGzip(path string) (io.ReadCloser, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return openMaybeGzipFile(f, path)
}

func openMaybeGzipFile(f *os.File, label string) (io.ReadCloser, error) {
	br := bufio.NewReaderSize(f, 512)
	magic, err := br.Peek(2)
	if err != nil {
		return &plainFile{Reader: br, f: f}, nil
	}

	if magic[0] == 0x1f && magic[1] == 0x8b {
		gz, err := gzip.NewReader(br)
		if err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("gzip open %s: %w", label, err)
		}
		return &gzipFile{Reader: gz, f: f}, nil
	}

	return &plainFile{Reader: br, f: f}, nil
}

type plainFile struct {
	io.Reader
	f *os.File
}

func (p *plainFile) Close() error { return p.f.Close() }

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
