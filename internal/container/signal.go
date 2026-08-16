package container

import (
	"fmt"
	"os"
	"runtime"
	"syscall"

	"minicontainer/internal/state"
)

// SendSignal sends a custom OS signal to a container's payload PID 1.
func SendSignal(st *state.Store, containerID, sigName string) error {
	if st == nil {
		return fmt.Errorf("state store is nil")
	}

	c, err := st.Resolve(containerID)
	if err != nil {
		return fmt.Errorf("resolve container: %w", err)
	}

	if c.Status != state.StatusRunning || c.PID <= 0 {
		return fmt.Errorf("container %s is not running", c.ID[:8])
	}

	proc, err := os.FindProcess(c.PID)
	if err != nil {
		return fmt.Errorf("find process %d: %w", c.PID, err)
	}

	if runtime.GOOS != "linux" {
		return nil
	}

	sig := syscall.SIGTERM
	if sigName == "SIGKILL" || sigName == "KILL" {
		sig = syscall.SIGKILL
	} else if sigName == "SIGHUP" || sigName == "HUP" {
		sig = syscall.SIGHUP
	}

	return proc.Signal(sig)
}
