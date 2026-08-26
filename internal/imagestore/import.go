package imagestore

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"minicontainer/internal/image"
	"minicontainer/internal/state"
)

type importRemoveAllFunc func(path string) error

// ImportRawRootFS unpacks a raw tarball into imagestore and tags it.
func ImportRawRootFS(st *state.Store, tarPath, imageTag string) (*state.Image, error) {
	return importRawRootFSWithCleanup(st, tarPath, imageTag, os.RemoveAll)
}

func importRawRootFSWithCleanup(st *state.Store, tarPath, imageTag string, removeAll importRemoveAllFunc) (result *state.Image, retErr error) {
	if st == nil {
		return nil, fmt.Errorf("state store is nil")
	}
	if imageTag == "" {
		return nil, fmt.Errorf("image tag cannot be empty")
	}
	if removeAll == nil {
		return nil, fmt.Errorf("image staging cleanup operation is nil")
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

	lease, err := st.AcquireImageStorage()
	if err != nil {
		return nil, fmt.Errorf("acquire image storage generation: %w", err)
	}
	defer func() {
		if err := lease.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("close image storage lease: %w", err))
		}
	}()

	sum := fmt.Sprintf("%x", h.Sum(nil))[:12]
	imagesDir := lease.Path()
	durableImagesDir := lease.DurablePath()
	imgDir := filepath.Join(imagesDir, sum)
	rootFS := filepath.Join(durableImagesDir, sum, "rootfs")

	// Build the rootfs out-of-place inside the exact image-directory generation
	// pinned by Store.Open. A replaced configured state pathname can therefore
	// never redirect extraction or publication into another filesystem tree.
	tmpDir, err := os.MkdirTemp(imagesDir, ".import-"+sum+"-")
	if err != nil {
		return nil, fmt.Errorf("create temporary image directory: %w", err)
	}
	cleanupPending := true
	cleanupStaging := func(context string) error {
		if !cleanupPending {
			return nil
		}
		if err := removeAll(tmpDir); err != nil {
			return fmt.Errorf("%s %q: %w", context, tmpDir, err)
		}
		cleanupPending = false
		return nil
	}
	defer func() {
		if !cleanupPending {
			return
		}
		if err := cleanupStaging("remove temporary image directory"); err != nil {
			result = nil
			retErr = errors.Join(retErr, err)
		}
	}()

	tmpRootFS := filepath.Join(tmpDir, "rootfs")
	if err := os.MkdirAll(tmpRootFS, 0755); err != nil {
		return nil, fmt.Errorf("create temporary rootfs: %w", err)
	}
	if err := image.Unpack(tarPath, tmpRootFS); err != nil {
		return nil, fmt.Errorf("unpack rootfs: %w", err)
	}

	publishedOwned := false
	if err := os.Rename(tmpDir, imgDir); err != nil {
		if _, statErr := os.Stat(imgDir); statErr != nil {
			return nil, fmt.Errorf("publish image rootfs: %w", err)
		}
		// Identical content may already have been imported concurrently or by an
		// earlier run. Before writing another metadata record, the private staging
		// directory must actually be gone; otherwise a successful import would
		// silently leave durable garbage behind.
		if cleanupErr := cleanupStaging("discard duplicate import staging"); cleanupErr != nil {
			return nil, cleanupErr
		}
	} else {
		// tmpDir has moved to imgDir, so there is no staging pathname left for
		// this call to clean up. This call exclusively owns the published path
		// until its image metadata is committed.
		cleanupPending = false
		publishedOwned = true
	}

	// RootFS metadata uses the configured durable path, not /proc/self/fd. Prove
	// immediately before metadata publication that the configured root/images
	// path still names this exact leased generation. If the pathname changed
	// while extraction was running, remove only content published by this call.
	if err := lease.ValidateConfiguredGeneration(); err != nil {
		boundaryErr := fmt.Errorf("validate image storage generation before metadata publication: %w", err)
		if publishedOwned {
			if cleanupErr := removeAll(imgDir); cleanupErr != nil {
				boundaryErr = errors.Join(boundaryErr, fmt.Errorf("rollback unpublished image rootfs %q: %w", imgDir, cleanupErr))
			}
		}
		return nil, boundaryErr
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
