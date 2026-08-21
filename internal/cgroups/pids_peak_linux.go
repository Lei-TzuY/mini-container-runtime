//go:build linux

package cgroups

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ReadPIDSPeak reads the maximum number of concurrent tasks recorded in pids.peak.
func ReadPIDSPeak(cgroupPath string) (uint64, error) {
	data, err := os.ReadFile(filepath.Join(cgroupPath, "pids.peak"))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	val := strings.TrimSpace(string(data))
	if val == "" || val == "max" {
		return 0, nil
	}
	return strconv.ParseUint(val, 10, 64)
}

// ResetPIDSPeak resets the pids.peak watermark by writing "0" to pids.peak.
func ResetPIDSPeak(cgroupPath string) error {
	return os.WriteFile(filepath.Join(cgroupPath, "pids.peak"), []byte("0\n"), 0644)
}
