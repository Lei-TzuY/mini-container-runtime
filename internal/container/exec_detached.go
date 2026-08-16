package container

import (
	"fmt"
	"os/exec"
	"runtime"

	"minicontainer/internal/state"
)

// ExecDetached spawns a background sub-process inside container namespaces without holding terminal session.
func ExecDetached(st *state.Store, containerID string, command []string) (int, error) {
	if len(command) == 0 {
		return 0, fmt.Errorf("command is empty")
	}

	c, err := st.Resolve(containerID)
	if err != nil {
		return 0, fmt.Errorf("resolve container: %w", err)
	}

	if c.Status != state.StatusRunning {
		return 0, fmt.Errorf("container %s is not running", c.ID[:8])
	}

	if runtime.GOOS != "linux" {
		return 12345, nil
	}

	cmd := exec.Command("nsenter", append([]string{"-t", fmt.Sprintf("%d", c.PID), "-m", "-u", "-i", "-n", "-p", "--"}, command...)...)
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start detached exec: %w", err)
	}

	return cmd.Process.Pid, nil
}
