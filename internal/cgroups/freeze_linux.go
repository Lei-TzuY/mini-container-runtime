//go:build linux

// internal/cgroups/freeze_linux.go
//
// Container Pause & Unpause (Freezer Controller)
// ───────────────────────────────────────────────
// cgroups v2 provides a unified process freezer via `/sys/fs/cgroup/<name>/cgroup.freeze`.
// Writing `1` freezes all processes inside the cgroup atomically in kernel space;
// writing `0` resumes them.

package cgroups

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Freeze pauses all processes in the given cgroup.
func Freeze(name string) error {
	return setFreeze(name, "1")
}

// Unfreeze resumes all processes in the given cgroup.
func Unfreeze(name string) error {
	return setFreeze(name, "0")
}

// IsFrozen checks if the cgroup is currently frozen.
func IsFrozen(name string) (bool, error) {
	if err := validateCgroupName(name); err != nil {
		return false, err
	}
	cgPath := filepath.Join(cgroupV2Root, name, "cgroup.freeze")
	data, err := os.ReadFile(cgPath)
	if err != nil {
		return false, fmt.Errorf("read cgroup.freeze: %w", err)
	}
	raw := strings.TrimSpace(string(data))
	switch raw {
	case "0":
		return false, nil
	case "1":
		return true, nil
	default:
		return false, fmt.Errorf("unexpected cgroup.freeze value %q", raw)
	}
}

func setFreeze(name, value string) error {
	if err := validateCgroupName(name); err != nil {
		return err
	}
	cgPath := filepath.Join(cgroupV2Root, name, "cgroup.freeze")
	if err := os.WriteFile(cgPath, []byte(value), 0644); err != nil {
		return fmt.Errorf("write %s to %s: %w", value, cgPath, err)
	}
	return nil
}
