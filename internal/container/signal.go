package container

import (
	"fmt"

	"minicontainer/internal/state"
)

// SendSignal sends a custom signal to the exact process identity persisted for
// a running container. It never falls back to signaling a raw numeric PID.
func SendSignal(st *state.Store, containerID, sigName string) error {
	c, handle, err := openRunningProcess(st, containerID)
	if err != nil {
		return err
	}
	defer handle.Close()

	sig, err := ParseSignal(sigName)
	if err != nil {
		return err
	}
	if err := handle.Signal(sig); err != nil {
		shortID := c.ID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}
		return fmt.Errorf("signal container %s: %w", shortID, err)
	}
	return nil
}
