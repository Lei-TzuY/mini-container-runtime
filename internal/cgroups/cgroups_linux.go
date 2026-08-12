//go:build linux

// internal/cgroups/cgroups_linux.go
//
// Control Groups (cgroups) — Resource Limits

package cgroups

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

const cgroupV2Root = "/sys/fs/cgroup"

// Config describes the resource limits to apply to a container's cgroup.
type Config struct {
	// Name is the cgroup subdirectory name, e.g. "minicontainer-1234".
	Name string

	// MemoryMax is the hard memory limit in bytes. 0 means unlimited.
	MemoryMax int64

	// CPUWeight is the relative CPU scheduling weight in the range 1–10000.
	CPUWeight int64

	// CPUs is the fractional CPU quota (e.g. 0.5 = 50% of 1 CPU, 2.0 = 2 CPUs).
	CPUs float64

	// PidsMax is the maximum number of processes (threads) inside the cgroup.
	PidsMax int64
}

// Apply creates a cgroup for pid and enforces the limits in cfg.
func Apply(pid int, cfg Config, debug bool) error {
	if isV2() {
		if debug {
			fmt.Println("[cgroup] using cgroup v2 (unified hierarchy)")
		}
		return applyV2(pid, cfg, debug)
	}
	if debug {
		fmt.Println("[cgroup] using cgroup v1 (legacy hierarchy)")
	}
	return applyV1(pid, cfg, debug)
}

func Remove(name string, debug bool) {
	if name == "" {
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

	if err := os.MkdirAll(cgPath, 0755); err != nil {
		return fmt.Errorf("mkdir cgroup %s: %w", cgPath, err)
	}

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

	if err := write("cgroup.procs", strconv.Itoa(pid)); err != nil {
		return err
	}

	if cfg.MemoryMax > 0 {
		if err := write("memory.max", strconv.FormatInt(cfg.MemoryMax, 10)); err != nil {
			return err
		}
		_ = write("memory.swap.max", "0")
	}

	if cfg.CPUWeight > 0 {
		if err := write("cpu.weight", strconv.FormatInt(cfg.CPUWeight, 10)); err != nil {
			return err
		}
	}

	// Hard CPU Quota (e.g. 0.5 CPUs = 50000 100000)
	if cfg.CPUs > 0 {
		periodUs := int64(100000) // 100ms default period
		quotaUs := int64(cfg.CPUs * float64(periodUs))
		val := fmt.Sprintf("%d %d", quotaUs, periodUs)
		if err := write("cpu.max", val); err != nil {
			if debug {
				fmt.Printf("[cgroup v2] write cpu.max: %v (ignored)\n", err)
			}
		}
	}

	if cfg.PidsMax > 0 {
		if err := write("pids.max", strconv.FormatInt(cfg.PidsMax, 10)); err != nil {
			return err
		}
	}

	return nil
}

func applyV1(pid int, cfg Config, debug bool) error {
	writeV1 := func(controller, file, value string) {
		cgPath := filepath.Join("/sys/fs/cgroup", controller, cfg.Name)
		if err := os.MkdirAll(cgPath, 0755); err != nil {
			if debug {
				fmt.Printf("[cgroup v1] mkdir %s: %v\n", cgPath, err)
			}
			return
		}
		path := filepath.Join(cgPath, file)
		if err := os.WriteFile(path, []byte(value), 0644); err != nil {
			if debug {
				fmt.Printf("[cgroup v1] write %s: %v\n", path, err)
			}
			return
		}
		if debug {
			fmt.Printf("[cgroup v1] %s/%s = %s\n", controller, file, value)
		}
	}

	pidStr := strconv.Itoa(pid)

	if cfg.MemoryMax > 0 {
		writeV1("memory", "memory.limit_in_bytes", strconv.FormatInt(cfg.MemoryMax, 10))
		writeV1("memory", "memory.memsw.limit_in_bytes", strconv.FormatInt(cfg.MemoryMax, 10))
		writeV1("memory", "tasks", pidStr)
	}

	if cfg.CPUWeight > 0 {
		writeV1("cpu", "cpu.shares", strconv.FormatInt(cfg.CPUWeight, 10))
		writeV1("cpu", "tasks", pidStr)
	}

	if cfg.CPUs > 0 {
		periodUs := int64(100000)
		quotaUs := int64(cfg.CPUs * float64(periodUs))
		writeV1("cpu", "cpu.cfs_period_us", strconv.FormatInt(periodUs, 10))
		writeV1("cpu", "cpu.cfs_quota_us", strconv.FormatInt(quotaUs, 10))
		writeV1("cpu", "tasks", pidStr)
	}

	if cfg.PidsMax > 0 {
		writeV1("pids", "pids.max", strconv.FormatInt(cfg.PidsMax, 10))
		writeV1("pids", "tasks", pidStr)
	}

	return nil
}
