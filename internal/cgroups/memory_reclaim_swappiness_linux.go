//go:build linux

package cgroups

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MemoryReclaimOptions holds advanced arguments for Cgroup v2 memory.reclaim.
type MemoryReclaimOptions struct {
	BytesToReclaim int64 // bytes to reclaim
	Swappiness     int   // -1 to ignore, 0-200 to enforce swap behavior
	NumaNode       int   // -1 to ignore, >=0 to specify NUMA node
}

// ReclaimMemoryWithOptions triggers Cgroup v2 memory.reclaim with optional swappiness and NUMA target.
func ReclaimMemoryWithOptions(cgroupPath string, opts MemoryReclaimOptions) error {
	if opts.BytesToReclaim <= 0 {
		opts.BytesToReclaim = 1048576 // 1MB default
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("%d", opts.BytesToReclaim))

	if opts.Swappiness >= 0 && opts.Swappiness <= 200 {
		parts = append(parts, fmt.Sprintf("swappiness=%d", opts.Swappiness))
	}
	if opts.NumaNode >= 0 {
		parts = append(parts, fmt.Sprintf("node=%d", opts.NumaNode))
	}

	cmd := strings.Join(parts, " ") + "\n"
	reclaimFile := filepath.Join(cgroupPath, "memory.reclaim")
	return os.WriteFile(reclaimFile, []byte(cmd), 0644)
}
