//go:build linux

package cgroups

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ReadCPUMaxBurst reads the burst quota budget in microseconds from cpu.max.burst.
func ReadCPUMaxBurst(cgroupPath string) (uint64, error) {
	data, err := os.ReadFile(filepath.Join(cgroupPath, "cpu.max.burst"))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	val := strings.TrimSpace(string(data))
	if val == "" {
		return 0, nil
	}
	return strconv.ParseUint(val, 10, 64)
}

// WriteCPUMaxBurst sets the CPU max burst quota in microseconds in cpu.max.burst.
func WriteCPUMaxBurst(cgroupPath string, burstUsec uint64) error {
	return os.WriteFile(filepath.Join(cgroupPath, "cpu.max.burst"),
		[]byte(fmt.Sprintf("%d\n", burstUsec)), 0644)
}
