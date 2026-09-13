//go:build linux

// internal/cgroups/cgroups_linux.go
//
// Control Groups (cgroups) — Resource Limits

package cgroups

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const cgroupV2Root = "/sys/fs/cgroup"

// Config describes the resource limits to apply to a container's cgroup.
type Config struct {
	// Name is the cgroup subdirectory name, e.g. "minicontainer-1234".
	Name string

	// MemoryMax is the hard memory limit in bytes. 0 means unlimited.
	MemoryMax int64

	// MemoryHigh is the cgroup v2 soft memory throttle threshold in bytes.
	// 0 leaves memory.high unchanged.
	MemoryHigh int64

	// MemorySwap is the OCI memory+swap total in bytes. 0 preserves the
	// runtime default of disabling swap when MemoryMax is set; -1 means
	// unlimited swap. Positive values must be at least MemoryMax.
	MemorySwap int64

	// CPUWeight is the relative CPU scheduling weight in the range 1–10000.
	CPUWeight int64

	// CPUs is the fractional CPU quota (e.g. 0.5 = 50% of 1 CPU, 2.0 = 2 CPUs).
	CPUs float64

	// CPUSetCPUs and CPUSetMems pin execution to cgroup v2 CPU/NUMA sets.
	CPUSetCPUs string
	CPUSetMems string

	// PidsMax is the maximum number of processes (threads) inside the cgroup.
	PidsMax int64
}

// Apply creates a cgroup for pid and enforces the limits in cfg.
func Apply(pid int, cfg Config, debug bool) error {
	if pid <= 0 {
		return fmt.Errorf("invalid cgroup target PID %d", pid)
	}
	if err := validateCgroupName(cfg.Name); err != nil {
		return err
	}
	if err := validateResourceValues(cfg.MemoryMax, cfg.CPUWeight, cfg.CPUs, cfg.PidsMax); err != nil {
		return err
	}
	if cfg.MemoryHigh < 0 {
		return fmt.Errorf("memory high must be non-negative")
	}
	if cfg.MemorySwap < -1 {
		return fmt.Errorf("memory swap total must be -1, zero, or positive")
	}
	if cfg.MemorySwap != 0 && cfg.MemoryMax <= 0 {
		return fmt.Errorf("memory swap total requires a positive memory max")
	}
	if cfg.MemorySwap > 0 && cfg.MemorySwap < cfg.MemoryMax {
		return fmt.Errorf("memory swap total %d is below memory max %d", cfg.MemorySwap, cfg.MemoryMax)
	}
	cfg.CPUSetCPUs = strings.TrimSpace(cfg.CPUSetCPUs)
	cfg.CPUSetMems = strings.TrimSpace(cfg.CPUSetMems)

	if isV2() {
		if debug {
			fmt.Println("[cgroup] using cgroup v2 (unified hierarchy)")
		}
		return applyV2(pid, cfg, debug)
	}
	if cfg.MemoryHigh > 0 {
		return fmt.Errorf("memory high resource control requires cgroup v2")
	}
	if cfg.MemorySwap != 0 {
		return fmt.Errorf("memory swap resource control requires cgroup v2")
	}
	if cfg.CPUSetCPUs != "" || cfg.CPUSetMems != "" {
		return fmt.Errorf("cpuset resource controls require cgroup v2")
	}
	if debug {
		fmt.Println("[cgroup] using cgroup v1 (legacy hierarchy)")
	}
	return applyV1(pid, cfg, debug)
}

func Remove(name string, debug bool) {
	if err := validateCgroupName(name); err != nil {
		if debug {
			fmt.Printf("[cgroup] refusing cleanup for invalid name %q: %v\n", name, err)
		}
		return
	}

	if isV2() {
		removePath(filepath.Join(cgroupV2Root, name), debug)
		return
	}

	for _, controller := range []string{"memory", "cpu", "pids"} {
		removePath(filepath.Join("/sys/fs/cgroup", controller, name), debug)
	}
}

func removePath(path string, debug bool) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) && debug {
		fmt.Printf("[cgroup] cleanup %s: %v\n", path, err)
	}
}

func isV2() bool {
	_, err := os.Stat(filepath.Join(cgroupV2Root, "cgroup.controllers"))
	return err == nil
}

func applyV2(pid int, cfg Config, debug bool) error {
	cgPath := filepath.Join(cgroupV2Root, cfg.Name)

	if err := os.Mkdir(cgPath, 0755); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("cgroup %s already exists; refusing to reuse stale cgroup", cgPath)
		}
		return fmt.Errorf("mkdir cgroup %s: %w", cgPath, err)
	}

	success := false
	defer func() {
		if !success {
			removePath(cgPath, debug)
		}
	}()

	if err := configureV2(cgPath, pid, cfg, debug); err != nil {
		return err
	}
	success = true
	return nil
}

// configureV2 writes every requested resource limit before cgroup.procs. This
// ordering is deliberate: a configuration failure must not admit the process
// into a partially configured cgroup.
func configureV2(cgPath string, pid int, cfg Config, debug bool) error {
	write := func(file, value string) error {
		path := filepath.Join(cgPath, file)
		if err := os.WriteFile(path, []byte(value), 0644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		if debug {
			fmt.Printf("[cgroup v2] %-22s = %s\n", file, value)
		}
		return nil
	}

	if cfg.MemoryMax > 0 {
		if err := write("memory.max", strconv.FormatInt(cfg.MemoryMax, 10)); err != nil {
			return err
		}
		swapPath := filepath.Join(cgPath, "memory.swap.max")
		if _, err := os.Stat(swapPath); err == nil {
			swapValue := "0"
			switch {
			case cfg.MemorySwap == -1:
				swapValue = "max"
			case cfg.MemorySwap > 0:
				swapValue = strconv.FormatInt(cfg.MemorySwap-cfg.MemoryMax, 10)
			}
			if err := write("memory.swap.max", swapValue); err != nil {
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect %s: %w", swapPath, err)
		} else if cfg.MemorySwap != 0 {
			return fmt.Errorf("memory swap resource control unavailable: %s does not exist", swapPath)
		}
	}

	if cfg.MemoryHigh > 0 {
		if err := write("memory.high", strconv.FormatInt(cfg.MemoryHigh, 10)); err != nil {
			return err
		}
	}

	if cfg.CPUWeight > 0 {
		if err := write("cpu.weight", strconv.FormatInt(cfg.CPUWeight, 10)); err != nil {
			return err
		}
	}

	if cfg.CPUs > 0 {
		periodUs := int64(100000)
		quotaUs := int64(cfg.CPUs * float64(periodUs))
		val := fmt.Sprintf("%d %d", quotaUs, periodUs)
		if err := write("cpu.max", val); err != nil {
			return err
		}
	}

	// Pinning is configured before process admission so a failed cpuset write can
	// never leave the payload attached to a partially configured cgroup.
	if cfg.CPUSetMems != "" {
		if err := write("cpuset.mems", cfg.CPUSetMems); err != nil {
			return err
		}
	}
	if cfg.CPUSetCPUs != "" {
		if err := write("cpuset.cpus", cfg.CPUSetCPUs); err != nil {
			return err
		}
	}

	if cfg.PidsMax > 0 {
		if err := write("pids.max", strconv.FormatInt(cfg.PidsMax, 10)); err != nil {
			return err
		}
	}

	if err := write("cgroup.procs", strconv.Itoa(pid)); err != nil {
		return err
	}

	return nil
}

func applyV1(pid int, cfg Config, debug bool) error {
	return applyV1At("/sys/fs/cgroup", pid, cfg, debug)
}
