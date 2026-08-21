package volume

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"minicontainer/internal/imagestore"
	"minicontainer/internal/state"
)

var validVolumeNameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

// Volume holds persistent volume metadata.
type Volume struct {
	Name      string    `json:"name"`
	MountPath string    `json:"mount_path"`
	CreatedAt time.Time `json:"created_at"`
	Size      int64     `json:"size"`
}

func DefaultVolumeDir() string {
	return filepath.Join(state.DefaultDir(), "volumes")
}

// ValidateVolumeName checks that the volume name adheres to alphanumeric naming
// conventions and does not escape the default volumes storage root.
func ValidateVolumeName(name string) error {
	if name == "" {
		return fmt.Errorf("volume name cannot be empty")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("invalid volume name %q: relative path components not allowed", name)
	}
	if strings.ContainsAny(name, "/\\:") {
		return fmt.Errorf("invalid volume name %q: path separators not allowed", name)
	}
	if !validVolumeNameRegex.MatchString(name) {
		return fmt.Errorf("invalid volume name %q: must start with alphanumeric character and contain only [a-zA-Z0-9_.-]", name)
	}

	volDir := filepath.Clean(filepath.Join(DefaultVolumeDir(), name))
	parentDir := filepath.Clean(DefaultVolumeDir())
	rel, err := filepath.Rel(parentDir, volDir)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("invalid volume name %q: path escapes volume directory", name)
	}
	return nil
}

// CreateVolume creates a new named persistent volume.
func CreateVolume(name string) (*Volume, error) {
	if err := ValidateVolumeName(name); err != nil {
		return nil, err
	}

	volDir := filepath.Join(DefaultVolumeDir(), name)
	dataPath := filepath.Join(volDir, "_data")

	if err := os.MkdirAll(dataPath, 0755); err != nil {
		return nil, fmt.Errorf("create volume data directory: %w", err)
	}

	vol := &Volume{
		Name:      name,
		MountPath: dataPath,
		CreatedAt: time.Now(),
	}

	metaPath := filepath.Join(volDir, "volume.json")
	data, err := json.MarshalIndent(vol, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal volume metadata: %w", err)
	}

	if err := os.WriteFile(metaPath, data, 0644); err != nil {
		return nil, fmt.Errorf("write volume metadata: %w", err)
	}

	return vol, nil
}

// GetVolume retrieves volume details by name.
func GetVolume(name string) (*Volume, error) {
	if err := ValidateVolumeName(name); err != nil {
		return nil, err
	}

	volDir := filepath.Join(DefaultVolumeDir(), name)
	metaPath := filepath.Join(volDir, "volume.json")

	data, err := os.ReadFile(metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("volume %q not found", name)
		}
		return nil, err
	}

	var vol Volume
	if err := json.Unmarshal(data, &vol); err != nil {
		return nil, err
	}

	sz, _ := imagestore.CalculateDirSize(vol.MountPath)
	vol.Size = sz
	return &vol, nil
}

// ListVolumes lists all registered volumes.
func ListVolumes() ([]*Volume, error) {
	dir := DefaultVolumeDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*Volume{}, nil
		}
		return nil, err
	}

	var out []*Volume
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if vol, err := GetVolume(entry.Name()); err == nil {
			out = append(out, vol)
		}
	}
	return out, nil
}

// RemoveVolume deletes a named volume.
func RemoveVolume(name string) error {
	if err := ValidateVolumeName(name); err != nil {
		return err
	}

	volDir := filepath.Join(DefaultVolumeDir(), name)
	if _, err := os.Stat(volDir); os.IsNotExist(err) {
		return fmt.Errorf("volume %q not found", name)
	}
	return os.RemoveAll(volDir)
}

// PruneVolumes removes all volumes.
func PruneVolumes() (int, error) {
	vols, err := ListVolumes()
	if err != nil {
		return 0, err
	}
	count := 0
	for _, v := range vols {
		if err := RemoveVolume(v.Name); err == nil {
			count++
		}
	}
	return count, nil
}

// ResolveVolumePath returns the host directory path for a volume name or host path.
func ResolveVolumePath(spec string) string {
	if err := ValidateVolumeName(spec); err == nil {
		vol, err := GetVolume(spec)
		if err == nil && vol.MountPath != "" {
			return vol.MountPath
		}
	}
	return spec
}
