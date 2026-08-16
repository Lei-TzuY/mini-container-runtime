package system

import (
	"fmt"

	"minicontainer/internal/imagestore"
	"minicontainer/internal/state"
	"minicontainer/internal/volume"
)

type DFResult struct {
	ContainersCount int   `json:"containers_count"`
	ContainersSize  int64 `json:"containers_size"`
	ImagesCount     int   `json:"images_count"`
	ImagesSize      int64 `json:"images_size"`
	VolumesCount    int   `json:"volumes_count"`
	VolumesSize     int64 `json:"volumes_size"`
}

type PruneResult struct {
	ContainersReclaimed int   `json:"containers_reclaimed"`
	ImagesReclaimed     int   `json:"images_reclaimed"`
	VolumesReclaimed    int   `json:"volumes_reclaimed"`
	SpaceReclaimed      int64 `json:"space_reclaimed"`
}

// CalculateDF computes engine storage usage across containers, images, and volumes.
func CalculateDF(st *state.Store) (*DFResult, error) {
	if st == nil {
		return nil, fmt.Errorf("state store is nil")
	}

	ctrs, err := st.List()
	if err != nil {
		return nil, err
	}

	imgs, err := st.ListImages()
	if err != nil {
		return nil, err
	}

	vols, err := volume.ListVolumes()
	if err != nil {
		return nil, err
	}

	res := &DFResult{
		ContainersCount: len(ctrs),
		ImagesCount:     len(imgs),
		VolumesCount:    len(vols),
	}

	for _, c := range ctrs {
		if c.RootFS != "" {
			sz, _ := imagestore.CalculateDirSize(c.RootFS)
			res.ContainersSize += sz
		}
	}

	for _, img := range imgs {
		sz := img.Size
		if sz == 0 && img.RootFS != "" {
			sz, _ = imagestore.CalculateDirSize(img.RootFS)
		}
		res.ImagesSize += sz
	}

	for _, v := range vols {
		res.VolumesSize += v.Size
	}

	return res, nil
}

// SystemPrune cleans up stopped containers, unused images, and unused volumes.
func SystemPrune(st *state.Store, pruneAll bool) (*PruneResult, error) {
	res := &PruneResult{}

	// Prune stopped containers
	ctrs, _ := st.List()
	for _, c := range ctrs {
		if c.Status == state.StatusStopped {
			if err := st.Delete(c.ID); err == nil {
				res.ContainersReclaimed++
			}
		}
	}

	// Prune unused volumes
	volsCount, _ := volume.PruneVolumes()
	res.VolumesReclaimed = volsCount

	// Prune images if all requested
	if pruneAll {
		imgs, _ := st.ListImages()
		for _, img := range imgs {
			if _, err := imagestore.RemoveImage(st, img.Name, true); err == nil {
				res.ImagesReclaimed++
			}
		}
	}

	return res, nil
}
