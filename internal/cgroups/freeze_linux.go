//go:build linux

// internal/cgroups/freeze_linux.go
//
// Container Pause & Unpause (Freezer Controller)
// ───────────────────────────────────────────────
// cgroups v2 provides a unified process freezer via `/sys/fs/cgroup/<name>/cgroup.freeze`.
// Writing `1` freezes all processes inside the cgroup atomically in kernel space;
// writing `0` resumes them.
//
// Unlike sending SIGSTOP to individual processes, freezing via cgroups:
//   • Operates atomically on the whole process sub-tree.
//   • Does not leak signals or trigger user-space signal handlers.
//   • Prevents new processes spawned inside from escaping freeze.

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
	cgPath := filepath.Join(cgroupV2Root, name, "cgroup.freeze")
	data, err := os.ReadFile(cgPath)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(data)) == "1", nil
}

func setFreeze(name, value string) error {
	cgPath := filepath.Join(cgroupV2Root, name, "cgroup.freeze")
	if err := os.WriteFile(cgPath, []byte(value), 0644); err != nil {
		return fmt.Errorf("write %s to %s: %w", value, cgPath, err)
	}
	return nil
}
