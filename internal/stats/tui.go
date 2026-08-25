package stats

import (
	"fmt"

	"minicontainer/internal/cgroups"
	"minicontainer/internal/container"
	"minicontainer/internal/state"
)

type ContainerStat struct {
	ContainerID    string            `json:"container_id"`
	PID            int               `json:"pid"`
	Status         string            `json:"status"`
	Available      bool              `json:"available"`
	CPUPercent     float64           `json:"cpu_percent"`
	CPUUsageUsec   uint64            `json:"cpu_usage_usec"`
	MemBytes       int64             `json:"mem_bytes"`
	MemLimitBytes  int64             `json:"mem_limit_bytes"`
	PIDs           int               `json:"pids"`
	CPUPressure    *cgroups.PSIStats `json:"cpu_pressure,omitempty"`
	MemoryPressure *cgroups.PSIStats `json:"memory_pressure,omitempty"`
	IOPressure     *cgroups.PSIStats `json:"io_pressure,omitempty"`
}

// CollectStats fetches current cgroup v2 metrics across all active containers.
// CPUPercent remains zero because a percentage requires two time-separated CPU
// usage samples; CPUUsageUsec exposes the lossless cumulative counter instead.
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
		if c.Status != state.StatusRunning || !container.IsRunning(c.PID) {
			continue
		}

		result := ContainerStat{
			ContainerID: c.ID,
			PID:         c.PID,
			Status:      string(c.Status),
		}

		cgName := fmt.Sprintf("minicontainer-%d", c.PID)
		if snapshot, err := cgroups.ReadStats(cgName); err == nil {
			result.Available = true
			result.CPUUsageUsec = snapshot.CPUUsageUsec
			result.MemBytes = snapshot.MemoryUsage
			result.MemLimitBytes = snapshot.MemoryLimit
			result.PIDs = int(snapshot.PidsCurrent)
			result.CPUPressure = snapshot.CPUPressure
			result.MemoryPressure = snapshot.MemoryPressure
			result.IOPressure = snapshot.IOPressure
		}

		results = append(results, result)
	}
	return results, nil
}
