package stats

import (
	"math"
	"testing"
	"time"

	"minicontainer/internal/state"
)

func TestCalculateCPUPercent(t *testing.T) {
	tests := []struct {
		name          string
		before, after uint64
		elapsed       time.Duration
		want          float64
	}{
		{"idle", 100, 100, 100 * time.Millisecond, 0},
		{"half core", 1000, 51000, 100 * time.Millisecond, 50},
		{"one core", 0, 100000, 100 * time.Millisecond, 100},
		{"multiple cores", 0, 250000, 100 * time.Millisecond, 250},
		{"counter regression", 1000, 999, 100 * time.Millisecond, 0},
		{"zero elapsed", 0, 1000, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateCPUPercent(tt.before, tt.after, tt.elapsed)
			if math.Abs(got-tt.want) > 0.000001 {
				t.Fatalf("calculateCPUPercent() = %f, want %f", got, tt.want)
			}
		})
	}
}

func TestCollectStatsSampledRejectsInvalidInterval(t *testing.T) {
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatalf("state.Open: %v", err)
	}
	for _, interval := range []time.Duration{0, -time.Millisecond} {
		if _, err := CollectStatsSampled(st, interval); err == nil {
			t.Fatalf("expected error for interval %s", interval)
		}
	}
}

func TestCollectStatsSampledReturnsImmediatelyWithoutRunningContainers(t *testing.T) {
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatalf("state.Open: %v", err)
	}

	start := time.Now()
	got, err := CollectStatsSampled(st, 2*time.Second)
	if err != nil {
		t.Fatalf("CollectStatsSampled: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no stats, got %d", len(got))
	}
	if elapsed := time.Since(start); elapsed >= time.Second {
		t.Fatalf("empty collection unnecessarily slept for %s", elapsed)
	}
}
