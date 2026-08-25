//go:build linux

// internal/cgroups/update_linux.go
//
// Dynamic Container Resource Updating (`minictl update`)
// ───────────────────────────────────────────────────────
// Dynamically adjusts running container cgroup v2 limits (memory, CPUs, CPU weight, PIDs limit)
// without stopping or restarting the container process.

package cgroups

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// UpdateConfig holds resource limits to update dynamically.
type UpdateConfig struct {
	MemoryMax int64
	CPUs      float64
	CPUWeight int64
	PidsMax   int64
}

// UpdateLimits dynamically modifies cgroup limits for a running container.
func UpdateLimits(cgroupName string, cfg UpdateConfig, debug bool) error {
	if err := validateCgroupName(cgroupName); err != nil {
		return err
	}
	if err := validateResourceValues(cfg.MemoryMax, cfg.CPUWeight, cfg.CPUs, cfg.PidsMax); err != nil {
		return err
	}

	cgPath := filepath.Join(cgroupV2Root, cgroupName)
	info, err := os.Stat(cgPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("cgroup %s does not exist", cgroupName)
		}
		return fmt.Errorf("stat cgroup %s: %w", cgroupName, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("cgroup path %s is not a directory", cgPath)
	}

	if cfg.MemoryMax > 0 {
		memMaxFile := filepath.Join(cgPath, "memory.max")
		if err := os.WriteFile(memMaxFile, []byte(strconv.FormatInt(cfg.MemoryMax, 10)), 0644); err != nil {
			return fmt.Errorf("update memory.max: %w", err)
		}
		if debug {
			fmt.Printf("[cgroup] dynamically updated memory.max = %d\n", cfg.MemoryMax)
		}
	}

	if cfg.CPUs > 0 {
		cpuMaxFile := filepath.Join(cgPath, "cpu.max")
		periodUs := int64(100000)
		quotaUs := int64(cfg.CPUs * float64(periodUs))
		val := fmt.Sprintf("%d %d", quotaUs, periodUs)
		if err := os.WriteFile(cpuMaxFile, []byte(val), 0644); err != nil {
			return fmt.Errorf("update cpu.max: %w", err)
		}
		if debug {
			fmt.Printf("[cgroup] dynamically updated cpu.max = %s\n", val)
		}
	}

	if cfg.CPUWeight > 0 {
		weightFile := filepath.Join(cgPath, "cpu.weight")
		if err := os.WriteFile(weightFile, []byte(strconv.FormatInt(cfg.CPUWeight, 10)), 0644); err != nil {
			return fmt.Errorf("update cpu.weight: %w", err)
		}
		if debug {
			fmt.Printf("[cgroup] dynamically updated cpu.weight = %d\n", cfg.CPUWeight)
		}
	}

	if cfg.PidsMax > 0 {
		pidsFile := filepath.Join(cgPath, "pids.max")
		if err := os.WriteFile(pidsFile, []byte(strconv.FormatInt(cfg.PidsMax, 10)), 0644); err != nil {
			return fmt.Errorf("update pids.max: %w", err)
		}
		if debug {
			fmt.Printf("[cgroup] dynamically updated pids.max = %d\n", cfg.PidsMax)
		}
	}

	return nil
}
