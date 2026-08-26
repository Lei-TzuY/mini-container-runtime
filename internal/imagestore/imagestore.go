package imagestore

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
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
func RemoveImage(st *state.Store, nameOrID string, removeRootFS bool) (result *state.Image, retErr error) {
	if st == nil {
		return nil, fmt.Errorf("state store is nil")
	}
	if !removeRootFS {
		return st.DeleteImage(nameOrID)
	}

	// Rootfs removal is destructive. Prove that the complete metadata index is
	// readable and internally consistent before pinning or mutating anything.
	snapshot, err := st.ListImages()
	if err != nil {
		return nil, fmt.Errorf("preflight image metadata before removal: %w", err)
	}
	if err := validateImageAliasRootFSConsistency(snapshot); err != nil {
		return nil, fmt.Errorf("preflight image alias ownership before removal: %w", err)
	}
	img, err := st.GetImage(nameOrID)
	if err != nil {
		return nil, err
	}
	if !imageSnapshotContains(snapshot, img) {
		return nil, fmt.Errorf("image %q changed during destructive preflight", nameOrID)
	}

	var lease *state.ImageStorageLease
	var pinned managedImageRootFSRemoval
	if img.RootFS != "" && imageRootFSLooksManaged(filepath.Join(st.Dir(), "images"), img.RootFS) {
		lease, err = st.AcquireImageStorage()
		if err != nil {
			return nil, fmt.Errorf("acquire managed image storage for removal: %w", err)
		}
		defer func() {
			if err := lease.Close(); err != nil {
				retErr = errors.Join(retErr, fmt.Errorf("close managed image storage lease: %w", err))
			}
		}()

		var managed bool
		pinned, managed, err = prepareManagedImageRootFSRemoval(lease, img)
		if err != nil {
			return nil, fmt.Errorf("pin managed image rootfs before metadata removal: %w", err)
		}
		if !managed || pinned == nil {
			return nil, fmt.Errorf("managed image rootfs %q could not be pinned", img.RootFS)
		}
		defer func() {
			if err := pinned.Close(); err != nil {
				retErr = errors.Join(retErr, fmt.Errorf("close pinned managed image rootfs: %w", err))
			}
		}()
	}

	removed, err := st.DeleteImageIfMatch(nameOrID, img)
	if err != nil {
		return nil, err
	}
	result = removed
	if removed.RootFS == "" {
		return result, nil
	}

	remaining, err := st.ListImages()
	if err != nil {
		return result, fmt.Errorf("verify image rootfs references after metadata removal: %w", err)
	}
	if err := validateImageAliasRootFSConsistency(remaining); err != nil {
		return result, fmt.Errorf("verify image alias ownership after metadata removal: %w", err)
	}
	if imageRootFSReferenced(remaining, removed.RootFS) {
		return result, nil
	}

	if pinned != nil {
		if err := pinned.Remove(); err != nil {
			return result, fmt.Errorf("remove managed image rootfs %q: %w", removed.RootFS, err)
		}
		return result, nil
	}
	if err := os.RemoveAll(removed.RootFS); err != nil {
		return result, fmt.Errorf("remove image rootfs %q: %w", removed.RootFS, err)
	}
	return result, nil
}
