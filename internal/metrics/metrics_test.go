package metrics

import (
	"strings"
	"testing"
	"time"

	"minicontainer/internal/cgroups"
	"minicontainer/internal/state"
	runtimestats "minicontainer/internal/stats"
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
	if !strings.Contains(out, "# TYPE minictl_container_pressure_stall_seconds_total counter") {
		t.Fatalf("Metrics missing PSI counter metadata:\n%s", out)
	}
}

func TestAppendResourceMetrics(t *testing.T) {
	full := &cgroups.PSIValues{Avg10: 0.25, Avg60: 0.50, Avg300: 0.75, Total: 250000}
	containerStats := []runtimestats.ContainerStat{
		{
			ContainerID:   "ctr-live",
			Available:     true,
			CPUUsageUsec:  1250000,
			MemBytes:      64 * 1024 * 1024,
			MemLimitBytes: 128 * 1024 * 1024,
			PIDs:          7,
			CPUPressure: &cgroups.PSIStats{
				Some: cgroups.PSIValues{Avg10: 1.25, Avg60: 2.50, Avg300: 3.75, Total: 500000},
			},
			MemoryPressure: &cgroups.PSIStats{
				Some: cgroups.PSIValues{Avg10: 4.00, Avg60: 5.00, Avg300: 6.00, Total: 1000000},
				Full: full,
			},
		},
		{
			ContainerID:  "ctr-unavailable",
			Available:    false,
			CPUUsageUsec: 9999999,
			MemBytes:     999,
		},
	}

	var sb strings.Builder
	appendResourceMetrics(&sb, containerStats)
	out := sb.String()

	checks := []string{
		`minictl_container_cpu_usage_seconds_total{id="ctr-live"} 1.250000`,
		`minictl_container_memory_usage_bytes{id="ctr-live"} 67108864`,
		`minictl_container_memory_limit_bytes{id="ctr-live"} 134217728`,
		`minictl_container_pids_current{id="ctr-live"} 7`,
		`minictl_container_pressure_avg_percent{id="ctr-live",resource="cpu",scope="some",window="10"} 1.250000`,
		`minictl_container_pressure_stall_seconds_total{id="ctr-live",resource="cpu",scope="some"} 0.500000`,
		`minictl_container_pressure_avg_percent{id="ctr-live",resource="memory",scope="full",window="300"} 0.750000`,
		`minictl_container_pressure_stall_seconds_total{id="ctr-live",resource="memory",scope="full"} 0.250000`,
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Fatalf("resource metrics missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "ctr-unavailable") {
		t.Fatalf("unavailable cgroup snapshot should not emit resource samples:\n%s", out)
	}
}
