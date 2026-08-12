package volume

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"minicontainer/internal/imagestore"
	"minicontainer/internal/state"
)

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

// CreateVolume creates a new named persistent volume.
func CreateVolume(name string) (*Volume, error) {
	if name == "" {
		return nil, fmt.Errorf("volume name cannot be empty")
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
	vol, err := GetVolume(spec)
	if err == nil && vol.MountPath != "" {
		return vol.MountPath
	}
	return spec
}
