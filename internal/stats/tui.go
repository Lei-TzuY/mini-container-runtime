package stats

import (
	"fmt"

	"minicontainer/internal/container"
	"minicontainer/internal/state"
)

type ContainerStat struct {
	ContainerID string  `json:"container_id"`
	PID         int     `json:"pid"`
	Status      string  `json:"status"`
	CPUPercent  float64 `json:"cpu_percent"`
	MemBytes    int64   `json:"mem_bytes"`
	PIDs        int     `json:"pids"`
}

// CollectStats fetches current metrics across all active containers.
func CollectStats(st *state.Store) ([]ContainerStat, error) {
	if st == nil {
		return nil, fmt.Errorf("state store is nil")
	}

	all, err := st.List()
	if err != nil {
		return nil, err
	}

	var results []ContainerStat
	for _, c := range all {
		if c.Status == state.StatusRunning && container.IsRunning(c.PID) {
			results = append(results, ContainerStat{
				ContainerID: c.ID,
				PID:         c.PID,
				Status:      string(c.Status),
				CPUPercent:  0.0,
				MemBytes:    0,
				PIDs:        1,
			})
		}
	}
	return results, nil
}
