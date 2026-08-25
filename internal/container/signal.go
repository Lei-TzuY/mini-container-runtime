package container

import (
	"errors"
	"fmt"

	"minicontainer/internal/state"
)

// SendSignal sends a custom signal to the exact process identity persisted for
// a running container. It never falls back to signaling a raw numeric PID.
func SendSignal(st *state.Store, containerID, sigName string) error {
	if st == nil {
		return fmt.Errorf("state store is nil")
	}

	c, err := st.Resolve(containerID)
	if err != nil {
		return fmt.Errorf("resolve container: %w", err)
	}
	shortID := c.ID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	if c.Status != state.StatusRunning {
		return fmt.Errorf("container %s is not running", shortID)
	}
	if c.PID <= 0 || c.PIDStartTime == 0 {
		return fmt.Errorf("container %s: %w", shortID, ErrProcessIdentityUnavailable)
	}

	sig, err := ParseSignal(sigName)
	if err != nil {
		return err
	}
	handle, err := OpenProcessHandle(c.PID, c.PIDStartTime)
	if err != nil {
		if errors.Is(err, ErrProcessNotFound) {
			return fmt.Errorf("container %s process exited: %w", shortID, err)
		}
		return fmt.Errorf("container %s process verification: %w", shortID, err)
	}
	defer handle.Close()

	if err := handle.Signal(sig); err != nil {
		return fmt.Errorf("signal container %s: %w", shortID, err)
	}
	return nil
}
