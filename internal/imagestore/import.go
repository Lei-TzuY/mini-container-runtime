package imagestore

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"minicontainer/internal/image"
	"minicontainer/internal/state"
)

// ImportRawRootFS unpacks a raw tarball into imagestore and tags it.
func ImportRawRootFS(st *state.Store, tarPath, imageTag string) (*state.Image, error) {
	if st == nil {
		return nil, fmt.Errorf("state store is nil")
	}
	if imageTag == "" {
		return nil, fmt.Errorf("image tag cannot be empty")
	}

	f, err := os.Open(tarPath)
	if err != nil {
		return nil, fmt.Errorf("open tarball: %w", err)
	}
	h := sha256.New()
	size, err := io.Copy(h, f)
	closeErr := f.Close()
	if err != nil {
		return nil, fmt.Errorf("hash tarball: %w", err)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close tarball after hashing: %w", closeErr)
	}

	sum := fmt.Sprintf("%x", h.Sum(nil))[:12]
	imagesDir := filepath.Join(st.Dir(), "images")
	imgDir := filepath.Join(imagesDir, sum)
	rootFS := filepath.Join(imgDir, "rootfs")

	if err := os.MkdirAll(imagesDir, 0755); err != nil {
		return nil, fmt.Errorf("create image store: %w", err)
	}

	// Build the rootfs out-of-place so a malformed/truncated archive can never
	// publish a half-extracted image directory. The temporary directory lives in
	// the same parent so the final rename is atomic on a single filesystem.
	tmpDir, err := os.MkdirTemp(imagesDir, ".import-"+sum+"-")
	if err != nil {
		return nil, fmt.Errorf("create temporary image directory: %w", err)
	}
	published := false
	defer func() {
		if !published {
			_ = os.RemoveAll(tmpDir)
		}
	}()

	tmpRootFS := filepath.Join(tmpDir, "rootfs")
	if err := os.MkdirAll(tmpRootFS, 0755); err != nil {
		return nil, fmt.Errorf("create temporary rootfs: %w", err)
	}
	if err := image.Unpack(tarPath, tmpRootFS); err != nil {
		return nil, fmt.Errorf("unpack rootfs: %w", err)
	}

	if err := os.Rename(tmpDir, imgDir); err != nil {
		if _, statErr := os.Stat(imgDir); statErr != nil {
			return nil, fmt.Errorf("publish image rootfs: %w", err)
		}
		// Identical content may already have been imported concurrently or by an
		// earlier run. Keep the already-published immutable content and discard our
		// private staging directory.
		_ = os.RemoveAll(tmpDir)
	} else {
		published = true
	}

	imgRec := &state.Image{
		ID:       sum,
		Name:     imageTag,
		Tag:      imageTag,
		RootFS:   rootFS,
		Size:     size,
		LoadedAt: time.Now(),
	}

	if err := st.SaveImage(imgRec); err != nil {
		return nil, fmt.Errorf("save image record: %w", err)
	}

	return imgRec, nil
}
