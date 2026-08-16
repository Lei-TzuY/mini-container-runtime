package system

import (
	"fmt"
	"time"

	"minicontainer/internal/state"
)

// ParseUntilDuration parses human duration strings like "24h", "168h", "30m".
func ParseUntilDuration(untilStr string) (time.Duration, error) {
	if untilStr == "" {
		return 0, nil
	}
	return time.ParseDuration(untilStr)
}

// PruneUntil cleans up containers older than the specified creation cutoff time.
func PruneUntil(st *state.Store, cutoff time.Time) (*PruneResult, error) {
	if st == nil {
		return nil, fmt.Errorf("state store is nil")
	}

	res := &PruneResult{}
	ctrs, err := st.List()
	if err != nil {
		return nil, err
	}

	for _, c := range ctrs {
		if c.Status == state.StatusStopped && c.CreatedAt.Before(cutoff) {
			if err := st.Delete(c.ID); err == nil {
				res.ContainersReclaimed++
			}
		}
	}

	return res, nil
}
