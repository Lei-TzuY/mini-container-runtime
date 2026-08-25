package container

import (
	"errors"
	"testing"
	"time"

	"minicontainer/internal/state"
)

func TestFinalizeStoppedGenerationPersistsStateEvenWhenCleanupFails(t *testing.T) {
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	snapshot := &state.Container{
		ID:           "ctr-finalize-cleanup-error",
		Status:       state.StatusRunning,
		PID:          4242,
		PIDStartTime: 99,
		CreatedAt:    time.Now(),
	}
	if err := st.Save(snapshot); err != nil {
		t.Fatal(err)
	}

	cleanupFailure := errors.New("cgroup still populated")
	var gotID string
	var gotPID int
	var gotStart uint64
	changed, err := finalizeStoppedGenerationWithCleanup(
		st,
		snapshot,
		-1,
		time.Now(),
		func(id string, pid int, start uint64) error {
			gotID, gotPID, gotStart = id, pid, start
			return cleanupFailure
		},
	)
	if !changed {
		t.Fatal("expected running state to transition to stopped")
	}
	if !errors.Is(err, cleanupFailure) {
		t.Fatalf("cleanup failure not preserved: %v", err)
	}
	if gotID != snapshot.ID || gotPID != snapshot.PID || gotStart != snapshot.PIDStartTime {
		t.Fatalf("cleanup targeted wrong generation: id=%q pid=%d start=%d", gotID, gotPID, gotStart)
	}

	current, err := st.Get(snapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != state.StatusStopped || current.PID != 0 || current.PIDStartTime != 0 {
		t.Fatalf("dead process left as running after cleanup failure: %+v", current)
	}
}

func TestFinalizeStoppedGenerationDoesNotClobberConcurrentRestart(t *testing.T) {
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	oldGeneration := &state.Container{
		ID:           "ctr-finalize-restart",
		Status:       state.StatusRunning,
		PID:          1111,
		PIDStartTime: 10,
		CreatedAt:    time.Now(),
	}
	newGeneration := &state.Container{
		ID:           oldGeneration.ID,
		Status:       state.StatusRunning,
		PID:          2222,
		PIDStartTime: 20,
		CreatedAt:    oldGeneration.CreatedAt,
	}
	if err := st.Save(newGeneration); err != nil {
		t.Fatal(err)
	}

	cleanupCalls := 0
	changed, err := finalizeStoppedGenerationWithCleanup(
		st,
		oldGeneration,
		-1,
		time.Now(),
		func(id string, pid int, start uint64) error {
			cleanupCalls++
			if id != oldGeneration.ID || pid != oldGeneration.PID || start != oldGeneration.PIDStartTime {
				t.Fatalf("cleanup targeted restarted generation: id=%q pid=%d start=%d", id, pid, start)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("finalize old generation: %v", err)
	}
	if changed {
		t.Fatal("stale finalizer overwrote concurrently restarted state")
	}
	if cleanupCalls != 1 {
		t.Fatalf("cleanup calls=%d, want 1 for old generation", cleanupCalls)
	}

	current, err := st.Get(oldGeneration.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != state.StatusRunning || current.PID != newGeneration.PID || current.PIDStartTime != newGeneration.PIDStartTime {
		t.Fatalf("restart state was clobbered: %+v", current)
	}
}
