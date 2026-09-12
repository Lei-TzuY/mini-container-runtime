package container

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"minicontainer/internal/state"
)

func newManagedDetachedExecCommand(executable, containerID string, command []string) *exec.Cmd {
	args := append([]string{"exec", containerID}, command...)
	cmd := exec.Command(executable, args...)
	cmd.Env = os.Environ()
	configureDetachedExecCommand(cmd)
	return cmd
}

// ExecDetached starts a background exec through the same managed minictl exec
// lifecycle as foreground exec. The child re-resolves and reconciles container
// state before entering the normal exec path, so detached workloads retain the
// same cgroup, process-policy, workdir, environment, and namespace semantics.
func ExecDetached(st *state.Store, containerID string, command []string) (int, error) {
	if len(command) == 0 || command[0] == "" {
		return 0, fmt.Errorf("command is empty")
	}

	c, err := st.Resolve(containerID)
	if err != nil {
		return 0, fmt.Errorf("resolve container: %w", err)
	}
	if c.Status != state.StatusRunning {
		return 0, fmt.Errorf("container %s is not running", c.ID[:min(8, len(c.ID))])
	}

	if runtime.GOOS != "linux" {
		return 12345, nil
	}

	self, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("resolve runtime executable: %w", err)
	}
	cmd := newManagedDetachedExecCommand(self, c.ID, command)
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start managed detached exec: %w", err)
	}
	pid := cmd.Process.Pid
	go func() { _ = cmd.Wait() }()
	return pid, nil
}
