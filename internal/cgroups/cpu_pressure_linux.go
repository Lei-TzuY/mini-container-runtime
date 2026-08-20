//go:build linux

package cgroups

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ReadCPUPressureStallTotal reads the total stall time (in microseconds)
// from the CPU PSI (Pressure Stall Information) interface.
// Returns the "total" value from the "some" line of cpu.pressure.
func ReadCPUPressureStallTotal(cgroupPath string) (uint64, error) {
	data, err := os.ReadFile(filepath.Join(cgroupPath, "cpu.pressure"))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read cpu.pressure: %w", err)
	}

	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "some" {
			continue
		}
		for _, field := range fields[1:] {
			if strings.HasPrefix(field, "total=") {
				valStr := strings.TrimPrefix(field, "total=")
				return strconv.ParseUint(valStr, 10, 64)
			}
		}
	}

	return 0, nil
}
