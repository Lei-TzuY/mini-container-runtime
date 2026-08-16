package stats

import (
	"testing"
	"time"

	"minicontainer/internal/state"
)

func TestCollectStats(t *testing.T) {
	tmpDir := t.TempDir()
	st, err := state.Open(tmpDir)
	if err != nil {
		t.Fatalf("Open state store error: %v", err)
	}

	c := &state.Container{
		ID:        "ctr-stats-1",
		PID:       99999, // non-running PID
		Status:    state.StatusStopped,
		CreatedAt: time.Now(),
	}
	_ = st.Save(c)

	stats, err := CollectStats(st)
	if err != nil {
		t.Fatalf("CollectStats error: %v", err)
	}

	if len(stats) != 0 {
		t.Fatalf("CollectStats should return 0 stats for stopped container")
	}
}
