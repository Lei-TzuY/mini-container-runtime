package metrics

import (
	"strings"
	"testing"
	"time"

	"minicontainer/internal/state"
)

func TestPrometheusMetrics(t *testing.T) {
	tmpDir := t.TempDir()
	st, err := state.Open(tmpDir)
	if err != nil {
		t.Fatalf("Open state store error: %v", err)
	}

	c := &state.Container{
		ID:        "ctr-metric-1",
		Hostname:  "metric-host",
		Status:    state.StatusRunning,
		Health:    "healthy",
		CreatedAt: time.Now(),
	}
	if err := st.Save(c); err != nil {
		t.Fatalf("Save container error: %v", err)
	}

	out, err := GeneratePrometheusMetrics(st)
	if err != nil {
		t.Fatalf("GeneratePrometheusMetrics error: %v", err)
	}

	if !strings.Contains(out, `minictl_container_status{id="ctr-metric-1"} 1`) {
		t.Fatalf("Metrics missing container status line:\n%s", out)
	}
	if !strings.Contains(out, "minictl_images_total 0") {
		t.Fatalf("Metrics missing images total line:\n%s", out)
	}
}
