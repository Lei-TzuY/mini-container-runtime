package imagestore

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"minicontainer/internal/state"
)

// GenerateImageID returns a random 12-hex-character ID for an image.
func GenerateImageID() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%012x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// CalculateDirSize recursively calculates total bytes of files inside path.
func CalculateDirSize(path string) (int64, error) {
	var total int64
	err := filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			info, err := d.Info()
			if err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total, err
}

// ParseRepositoryTag splits "ubuntu:22.04" into ("ubuntu", "22.04")
func ParseRepositoryTag(imageName string) (string, string) {
	if strings.Contains(imageName, ":") {
		parts := strings.SplitN(imageName, ":", 2)
		return parts[0], parts[1]
	}
	return imageName, "latest"
}

// TagImage creates an alias tag for an existing image.
func TagImage(st *state.Store, source, target string) (*state.Image, error) {
	img, err := st.GetImage(source)
	if err != nil {
		return nil, fmt.Errorf("source image %q not found: %w", source, err)
	}

	repo, tag := ParseRepositoryTag(target)
	newImg := *img
	newImg.Name = target
	newImg.Repository = repo
	newImg.Tag = tag

	if err := st.SaveImage(&newImg); err != nil {
		return nil, fmt.Errorf("save tagged image %q: %w", target, err)
	}
	return &newImg, nil
}

// RemoveImage removes image metadata and optionally cleans up the rootfs folder if no other tags reference it.
func RemoveImage(st *state.Store, nameOrID string, removeRootFS bool) (*state.Image, error) {
	img, err := st.DeleteImage(nameOrID)
	if err != nil {
		return nil, err
	}

	if removeRootFS && img.RootFS != "" {
		all, _ := st.ListImages()
		inUse := false
		for _, other := range all {
			if other.RootFS == img.RootFS {
				inUse = true
				break
			}
		}
		if !inUse {
			_ = os.RemoveAll(img.RootFS)
		}
	}
	return img, nil
}
