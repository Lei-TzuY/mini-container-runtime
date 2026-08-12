//go:build linux

// internal/cgroups/stats_linux.go
//
// Resource Metrics & Monitoring
// ──────────────────────────────
// Container stats monitoring reads live usage figures directly from Linux
// cgroup v2 pseudo-files:
//
//   • Memory Usage : /sys/fs/cgroup/<name>/memory.current
//   • Memory Limit : /sys/fs/cgroup/<name>/memory.max ("max" = no limit)
//   • Process Count: /sys/fs/cgroup/<name>/pids.current
//   • CPU Usage    : /sys/fs/cgroup/<name>/cpu.stat (usage_usec field)
//
// This is how `docker stats` obtains per-container CPU%, Memory, and PID figures.

package cgroups

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Stats holds a snapshot of container resource metrics.
type Stats struct {
	MemoryUsage int64 // bytes currently used
	MemoryLimit int64 // max allowed bytes (0 = unlimited / host limit)
	PidsCurrent int64 // number of active processes/threads
	CPUUsageUsec uint64 // total CPU time consumed in microseconds
}

// ReadStats reads live cgroup metrics for the given cgroup name (e.g., "minicontainer-1234").
func ReadStats(name string) (*Stats, error) {
	cgPath := filepath.Join(cgroupV2Root, name)
	if _, err := os.Stat(cgPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("cgroup %s does not exist", name)
	}

	stats := &Stats{}

	// Memory usage
	if data, err := os.ReadFile(filepath.Join(cgPath, "memory.current")); err == nil {
		stats.MemoryUsage, _ = strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	}

	// Memory max limit
	if data, err := os.ReadFile(filepath.Join(cgPath, "memory.max")); err == nil {
		raw := strings.TrimSpace(string(data))
		if raw != "max" {
			stats.MemoryLimit, _ = strconv.ParseInt(raw, 10, 64)
		}
	}

	// Current PID count
	if data, err := os.ReadFile(filepath.Join(cgPath, "pids.current")); err == nil {
		stats.PidsCurrent, _ = strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	}

	// CPU usage (microseconds from cpu.stat)
	if f, err := os.Open(filepath.Join(cgPath, "cpu.stat")); err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) == 2 && fields[0] == "usage_usec" {
				stats.CPUUsageUsec, _ = strconv.ParseUint(fields[1], 10, 64)
				break
			}
		}
	}

	return stats, nil
}
