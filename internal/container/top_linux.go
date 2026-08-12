//go:build linux

// internal/container/top_linux.go
//
// Container Process Listing (`minictl top`)
// ──────────────────────────────────────────
// `docker top` lists processes running inside a container.
//
// Mechanism:
// We examine `/proc/<containerPID>/task` to find threads/tasks belonging to the
// container init process and read `/proc/<tid>/status` for thread details (PID, State, Name).

package container

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ProcessInfo holds information about a single process/thread inside the container.
type ProcessInfo struct {
	PID   int
	PPID  int
	Name  string
	State string
}

// GetContainerProcesses inspects tasks under /proc/<pid>/task to list processes.
func GetContainerProcesses(containerPID int) ([]ProcessInfo, error) {
	taskDir := fmt.Sprintf("/proc/%d/task", containerPID)
	entries, err := os.ReadDir(taskDir)
	if err != nil {
		return nil, fmt.Errorf("read task dir for PID %d: %w", containerPID, err)
	}

	var procs []ProcessInfo
	for _, entry := range entries {
		tid := entry.Name()
		statusPath := filepath.Join("/proc", containerPIDStr(containerPID), "task", tid, "status")
		f, err := os.Open(statusPath)
		if err != nil {
			continue
		}

		info := ProcessInfo{}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])

			switch key {
			case "Name":
				info.Name = val
			case "State":
				info.State = val
			case "Pid":
				info.PID, _ = strconvAtoi(val)
			case "PPid":
				info.PPID, _ = strconvAtoi(val)
			}
		}
		f.Close()

		if info.PID > 0 {
			procs = append(procs, info)
		}
	}

	return procs, nil
}

func containerPIDStr(pid int) string {
	return fmt.Sprintf("%d", pid)
}

func strconvAtoi(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}
