package imagestore

import (
	"crypto/sha256"
	"fmt"
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

	content, err := os.ReadFile(tarPath)
	if err != nil {
		return nil, fmt.Errorf("read tarball: %w", err)
	}

	sum := fmt.Sprintf("%x", sha256.Sum256(content))[:12]
	imgDir := filepath.Join(st.Dir(), "images", sum)
	rootFS := filepath.Join(imgDir, "rootfs")

	if err := os.MkdirAll(rootFS, 0755); err != nil {
		return nil, err
	}

	if err := image.Unpack(tarPath, rootFS); err != nil {
		return nil, fmt.Errorf("unpack rootfs: %w", err)
	}

	imgRec := &state.Image{
		ID:        sum,
		Name:      imageTag,
		Tag:       imageTag,
		RootFS:    rootFS,
		Size:      int64(len(content)),
		LoadedAt: time.Now(),
	}

	if err := st.SaveImage(imgRec); err != nil {
		return nil, fmt.Errorf("save image record: %w", err)
	}

	return imgRec, nil
}
