package imagestore

import (
	"fmt"

	"minicontainer/internal/state"
)

// PruneOrphanLayers scans the imagestore and deletes unused image records.
func PruneOrphanLayers(st *state.Store) (int, int64, error) {
	if st == nil {
		return 0, 0, fmt.Errorf("state store is nil")
	}

	images, err := st.ListImages()
	if err != nil {
		return 0, 0, err
	}

	var count int
	var reclaimedBytes int64

	for _, img := range images {
		if img.Tag == "" || img.Tag == "<none>" {
			targetKey := img.Name
			if targetKey == "" {
				targetKey = img.ID
			}
			if _, err := st.DeleteImage(targetKey); err == nil {
				count++
				reclaimedBytes += img.Size
			}
		}
	}

	return count, reclaimedBytes, nil
}
