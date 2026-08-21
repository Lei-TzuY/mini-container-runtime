package health

import (
	"context"
	"testing"
	"time"

	"minicontainer/internal/state"
)

func TestHealthSupervisor(t *testing.T) {
	tmpDir := t.TempDir()
	st, err := state.Open(tmpDir)
	if err != nil {
		t.Fatalf("Open state store error: %v", err)
	}

	ctr := &state.Container{
		ID:        "ctr-health-test",
		Status:    state.StatusRunning,
		Health:    StatusStarting,
		RootFS:    tmpDir,
		CreatedAt: time.Now(),
	}
	if err := st.Save(ctr); err != nil {
		t.Fatalf("Save container error: %v", err)
	}

	failCount := 0
	checkFn := func(ctx context.Context) (int, error) {
		if failCount < 2 {
			failCount++
			return 1, nil
		}
		return 0, nil
	}

	sup := NewSupervisor("ctr-health-test", Config{
		Interval: 10 * time.Millisecond,
		Timeout:  50 * time.Millisecond,
		Retries:  3,
	}, checkFn, st)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go sup.Start(ctx)

	for {
		updated, err := st.Get("ctr-health-test")
		if err == nil && updated.Health == StatusHealthy {
			return // Success
		}
		select {
		case <-ctx.Done():
			if err != nil {
				t.Fatalf("Get container error: %v", err)
			}
			t.Fatalf("Expected container status %s within deadline, got %s", StatusHealthy, updated.Health)
		case <-time.After(10 * time.Millisecond):
		}
	}
}
