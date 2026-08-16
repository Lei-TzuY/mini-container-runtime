package container

import (
	"fmt"
	"time"

	"minicontainer/internal/state"
)

// WaitContainer blocks until the specified container terminates and returns its exit code.
func WaitContainer(st *state.Store, containerID string) (int, error) {
	if st == nil {
		return -1, fmt.Errorf("state store is nil")
	}

	for {
		c, err := st.Resolve(containerID)
		if err != nil {
			return -1, fmt.Errorf("resolve container: %w", err)
		}

		if c.Status == state.StatusStopped {
			return c.ExitCode, nil
		}

		time.Sleep(10 * time.Millisecond)
	}
}
