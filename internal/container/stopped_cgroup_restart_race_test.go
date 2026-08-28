package container

import (
	"testing"
	"time"

	"minicontainer/internal/state"
)

func TestCleanupStoppedCgroupPreservesOwnershipDuringConcurrentRestart(t *testing.T) {
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	oldGeneration := &state.Container{
		ID:           "ctr-stopped-cleanup-restart-race",
		Status:       state.StatusRunning,
		PID:          4101,
		PIDStartTime: 51,
		CreatedAt:    time.Now(),
	}
	ownership := persistOwnedGeneration(t, st, oldGeneration)
	changed, err := st.MarkStoppedIfIdentity(oldGeneration.ID, oldGeneration.PID, oldGeneration.PIDStartTime, 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("old generation did not transition to stopped")
	}
	staleStopped, err := st.Get(oldGeneration.ID)
	if err != nil {
		t.Fatal(err)
	}

	const newPID = 4202
	const newStart = uint64(62)
	if err := st.MarkRunning(oldGeneration.ID, newPID, newStart, time.Now()); err != nil {
		t.Fatal(err)
	}

	cleanupCalls := 0
	if err := cleanupStoppedCgroupWithCleanup(st, staleStopped, func(string, int, uint64) error {
		cleanupCalls++
		return nil
	}); err != nil {
		t.Fatalf("stale stopped cleanup should be a no-op during restart: %v", err)
	}
	if cleanupCalls != 0 {
		t.Fatalf("stale stopped cleanup performed %d destructive cleanup call(s)", cleanupCalls)
	}

	gotOwnership, ok, err := st.GetCgroupOwnership(oldGeneration.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || gotOwnership != ownership {
		t.Fatalf("restart race consumed durable old-generation recovery proof: got=%+v ok=%v want=%+v", gotOwnership, ok, ownership)
	}
	current, err := st.Get(oldGeneration.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != state.StatusRunning || current.PID != newPID || current.PIDStartTime != newStart {
		t.Fatalf("restart lifecycle state changed during stale cleanup: %+v", current)
	}
}
