package stats

import (
	"os"
	"testing"
	"time"

	"minicontainer/internal/state"
)

func TestCollectStatsRejectsNilStore(t *testing.T) {
	if _, err := CollectStats(nil); err == nil {
		t.Fatal("expected nil store error")
	}
}

func TestCollectStatsSkipsStoppedContainer(t *testing.T) {
	tmpDir := t.TempDir()
	st, err := state.Open(tmpDir)
	if err != nil {
		t.Fatalf("Open state store error: %v", err)
	}

	c := &state.Container{
		ID:        "ctr-stats-stopped",
		PID:       99999,
		Status:    state.StatusStopped,
		CreatedAt: time.Now(),
	}
	if err := st.Save(c); err != nil {
		t.Fatalf("save stopped container: %v", err)
	}

	got, err := CollectStats(st)
	if err != nil {
		t.Fatalf("CollectStats error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("CollectStats should return 0 stats for stopped container, got %d", len(got))
	}
}

func TestCollectStatsKeepsRunningContainerWhenCgroupSnapshotUnavailable(t *testing.T) {
	tmpDir := t.TempDir()
	st, err := state.Open(tmpDir)
	if err != nil {
		t.Fatalf("Open state store error: %v", err)
	}

	c := &state.Container{
		ID:        "ctr-stats-running",
		PID:       os.Getpid(),
		Status:    state.StatusRunning,
		CreatedAt: time.Now(),
	}
	if err := st.Save(c); err != nil {
		t.Fatalf("save running container: %v", err)
	}

	got, err := CollectStats(st)
	if err != nil {
		t.Fatalf("CollectStats error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 running container stat, got %d", len(got))
	}
	if got[0].ContainerID != c.ID || got[0].PID != c.PID || got[0].Status != string(state.StatusRunning) {
		t.Fatalf("unexpected stat identity: %+v", got[0])
	}
	if got[0].PIDs < 0 || got[0].MemBytes < 0 || got[0].MemLimitBytes < 0 {
		t.Fatalf("resource counters must not be negative: %+v", got[0])
	}
}
