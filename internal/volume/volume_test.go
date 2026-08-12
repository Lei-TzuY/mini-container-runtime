package volume

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVolumeManagement(t *testing.T) {
	// Set temp home directory for testing
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	volName := "app-db-data"
	vol, err := CreateVolume(volName)
	if err != nil {
		t.Fatalf("CreateVolume error: %v", err)
	}

	if vol.Name != volName {
		t.Fatalf("Volume name = %s, want %s", vol.Name, volName)
	}

	// Write test file to volume mountpath
	testFile := filepath.Join(vol.MountPath, "data.db")
	if err := os.WriteFile(testFile, []byte("sqlite database data"), 0644); err != nil {
		t.Fatalf("Write test file error: %v", err)
	}

	fetched, err := GetVolume(volName)
	if err != nil || fetched.Size == 0 {
		t.Fatalf("GetVolume error: %v, size: %d", err, fetched.Size)
	}

	vols, err := ListVolumes()
	if err != nil || len(vols) != 1 {
		t.Fatalf("ListVolumes count = %d, want 1", len(vols))
	}

	resolvedPath := ResolveVolumePath(volName)
	if resolvedPath != vol.MountPath {
		t.Fatalf("ResolveVolumePath(%s) = %s, want %s", volName, resolvedPath, vol.MountPath)
	}

	if err := RemoveVolume(volName); err != nil {
		t.Fatalf("RemoveVolume error: %v", err)
	}

	if _, err := GetVolume(volName); err == nil {
		t.Fatalf("Volume %s should have been deleted", volName)
	}
}
