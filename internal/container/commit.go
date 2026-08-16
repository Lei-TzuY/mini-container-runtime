package container

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"minicontainer/internal/image"
	"minicontainer/internal/state"
)

// CommitContainer creates a new image from a container's current rootfs state.
func CommitContainer(st *state.Store, containerID, targetTag string) (*state.Image, error) {
	if st == nil {
		return nil, fmt.Errorf("state store is nil")
	}
	if targetTag == "" {
		return nil, fmt.Errorf("target tag cannot be empty")
	}

	c, err := st.Resolve(containerID)
	if err != nil {
		return nil, fmt.Errorf("resolve container: %w", err)
	}

	idBytes := sha256.Sum256([]byte(fmt.Sprintf("%s-%d", c.ID, time.Now().UnixNano())))
	imgID := fmt.Sprintf("%x", idBytes)[:12]

	imgDir := filepath.Join(st.Dir(), "images", imgID)
	rootFS := filepath.Join(imgDir, "rootfs")
	if err := os.MkdirAll(rootFS, 0755); err != nil {
		return nil, fmt.Errorf("create rootfs dir: %w", err)
	}

	tarPath := filepath.Join(imgDir, "layer.tar.gz")
	if err := image.ExportDir(c.RootFS, tarPath); err != nil {
		return nil, fmt.Errorf("export container rootfs: %w", err)
	}

	if err := image.Unpack(tarPath, rootFS); err != nil {
		return nil, fmt.Errorf("unpack committed layer: %w", err)
	}

	img := &state.Image{
		ID:       imgID,
		Name:     targetTag,
		Tag:      targetTag,
		RootFS:   rootFS,
		LoadedAt: time.Now(),
	}

	if err := st.SaveImage(img); err != nil {
		return nil, fmt.Errorf("save image record: %w", err)
	}

	return img, nil
}
