package container

import (
	"fmt"

	"minicontainer/internal/state"
)

// RenameContainer updates a container's hostname/alias.
func RenameContainer(st *state.Store, containerID, newName string) error {
	if st == nil {
		return fmt.Errorf("state store is nil")
	}
	if newName == "" {
		return fmt.Errorf("new container name cannot be empty")
	}

	c, err := st.Resolve(containerID)
	if err != nil {
		return fmt.Errorf("resolve container: %w", err)
	}

	c.Hostname = newName
	return st.Save(c)
}
