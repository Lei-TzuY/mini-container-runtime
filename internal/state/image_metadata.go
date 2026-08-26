package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
)

func imageStorageKey(img *Image) (string, error) {
	if img == nil {
		return "", fmt.Errorf("image state is nil")
	}
	key := img.Name
	if key == "" {
		key = img.ID
	}
	if err := validateImageSelector(key); err != nil {
		return "", err
	}
	return key, nil
}

// imageMetadataFilename maps the logical image metadata key to a fixed-length,
// collision-resistant filename. The full SHA-256 digest avoids collisions
// introduced by the historical sanitizeImageFilename replacement scheme.
func imageMetadataFilename(key string) string {
	sum := sha256.Sum256([]byte(key))
	return "img-" + hex.EncodeToString(sum[:]) + ".json"
}

func legacyImageMetadataFilename(key string) string {
	return sanitizeImageFilename(key) + ".json"
}

func readImageMetadata(path string) (*Image, error) {
	data, err := readRegularStateFile(path, "image state")
	if err != nil {
		return nil, err
	}
	var img Image
	if err := json.Unmarshal(data, &img); err != nil {
		return nil, fmt.Errorf("unmarshal image state %q: %w", filepath.Base(path), err)
	}
	if _, err := imageStorageKey(&img); err != nil {
		return nil, fmt.Errorf("invalid image state %q: %w", filepath.Base(path), err)
	}
	return &img, nil
}

func imageMetadataOwnedBy(path, key string) (bool, error) {
	img, err := readImageMetadata(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	storedKey, err := imageStorageKey(img)
	if err != nil {
		return false, err
	}
	return storedKey == key, nil
}

// saveImageMetadataUnlocked writes the new collision-resistant metadata first,
// then removes a historical sanitized-name file only if its contents prove it
// belongs to the same logical image. A colliding legacy file for another image
// is never removed.
func (s *Store) saveImageMetadataUnlocked(img *Image, data []byte) error {
	key, err := imageStorageKey(img)
	if err != nil {
		return err
	}
	if err := s.ensureImageNotPendingCleanupUnlocked(img); err != nil {
		return fmt.Errorf("refuse image metadata publication during pending cleanup: %w", err)
	}
	newPath := filepath.Join(s.imgDir, imageMetadataFilename(key))
	legacyPath := filepath.Join(s.imgDir, legacyImageMetadataFilename(key))

	migrateLegacy := false
	if legacyPath != newPath {
		owned, err := imageMetadataOwnedBy(legacyPath, key)
		if err != nil {
			return fmt.Errorf("inspect legacy image metadata for %q: %w", key, err)
		}
		migrateLegacy = owned
	}

	if err := atomicWriteFile(s.imgDir, newPath, data); err != nil {
		return err
	}
	if migrateLegacy {
		if err := removeStateFileDurable(s.imgDir, legacyPath, "legacy image metadata"); err != nil {
			return err
		}
	}
	return nil
}

// removeImageMetadataUnlocked removes both current and legacy metadata formats,
// but only after verifying each existing file belongs to the selected logical
// image. This prevents a sanitized-name collision from deleting another image.
func (s *Store) removeImageMetadataUnlocked(img *Image) error {
	key, err := imageStorageKey(img)
	if err != nil {
		return err
	}
	paths := []string{
		filepath.Join(s.imgDir, imageMetadataFilename(key)),
		filepath.Join(s.imgDir, legacyImageMetadataFilename(key)),
	}
	seenPaths := make(map[string]bool, len(paths))
	var errs []error
	for _, path := range paths {
		if seenPaths[path] {
			continue
		}
		seenPaths[path] = true
		owned, err := imageMetadataOwnedBy(path, key)
		if err != nil {
			errs = append(errs, fmt.Errorf("inspect image metadata %q: %w", filepath.Base(path), err))
			continue
		}
		if !owned {
			continue
		}
		if err := removeStateFileDurable(s.imgDir, path, "image metadata"); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

type seenImageMetadata struct {
	img     *Image
	current bool
	index   int
}

func isCurrentImageMetadataPath(path, key string) bool {
	return filepath.Base(path) == imageMetadataFilename(key)
}

// appendUniqueImageMetadata handles the migration window where both the old
// sanitized file and the new hash-keyed file can coexist. A correctly named new
// file is authoritative for that logical key; this prevents readers from
// failing during an update between the durable new write and legacy cleanup.
// Conflicting non-current duplicates remain corruption and fail closed.
func appendUniqueImageMetadata(out []*Image, seen map[string]seenImageMetadata, img *Image, path string) ([]*Image, error) {
	key, err := imageStorageKey(img)
	if err != nil {
		return nil, err
	}
	current := isCurrentImageMetadataPath(path, key)
	previous, ok := seen[key]
	if !ok {
		seen[key] = seenImageMetadata{img: img, current: current, index: len(out)}
		return append(out, img), nil
	}

	switch {
	case previous.current && current:
		if !reflect.DeepEqual(previous.img, img) {
			return nil, fmt.Errorf("conflicting current image metadata for %q", key)
		}
		return out, nil
	case previous.current && !current:
		return out, nil
	case !previous.current && current:
		out[previous.index] = img
		seen[key] = seenImageMetadata{img: img, current: true, index: previous.index}
		return out, nil
	default:
		if !reflect.DeepEqual(previous.img, img) {
			return nil, fmt.Errorf("conflicting duplicate image metadata for %q", key)
		}
		return out, nil
	}
}
