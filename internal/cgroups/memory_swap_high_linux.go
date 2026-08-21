//go:build linux

package cgroups

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ReadMemorySwapCurrent reads the current swap usage in bytes from memory.swap.current.
func ReadMemorySwapCurrent(cgroupPath string) (uint64, error) {
	data, err := os.ReadFile(filepath.Join(cgroupPath, "memory.swap.current"))
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

// ReadMemorySwapHigh reads the swap high watermark from memory.swap.high.
// Returns "max" as 0 with a special flag.
func ReadMemorySwapHigh(cgroupPath string) (uint64, bool, error) {
	data, err := os.ReadFile(filepath.Join(cgroupPath, "memory.swap.high"))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, true, nil // treat missing as "max"
		}
		return 0, false, err
	}
	val := strings.TrimSpace(string(data))
	if val == "max" || val == "" {
		return 0, true, nil
	}
	v, err := strconv.ParseUint(val, 10, 64)
	return v, false, err
}

// WriteMemorySwapHigh sets the swap high watermark in bytes, or "max" for unlimited.
func WriteMemorySwapHigh(cgroupPath string, limitBytes int64) error {
	var content string
	if limitBytes < 0 {
		content = "max\n"
	} else {
		content = fmt.Sprintf("%d\n", limitBytes)
	}
	return os.WriteFile(filepath.Join(cgroupPath, "memory.swap.high"), []byte(content), 0644)
}
