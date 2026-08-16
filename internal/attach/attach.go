package attach

import (
	"fmt"
	"io"
	"os"

	"minicontainer/internal/logs"
	"minicontainer/internal/state"
)

// AttachContainer attaches stdio streams to a running container's output log stream.
func AttachContainer(st *state.Store, containerID string, in io.Reader, out io.Writer) error {
	c, err := st.Resolve(containerID)
	if err != nil {
		return fmt.Errorf("resolve container: %w", err)
	}

	if c.Status != state.StatusRunning {
		return fmt.Errorf("container %s is not running (status: %s)", c.ID, c.Status)
	}

	logPath := logs.LogFilePath(c.ID)
	logFile, err := os.Open(logPath)
	if err != nil {
		return fmt.Errorf("open container log stream: %w", err)
	}
	defer logFile.Close()

	fmt.Fprintf(out, "You are attached to container %s. Press Ctrl+C to detach.\n", c.ID[:min(8, len(c.ID))])

	_, err = io.Copy(out, logFile)
	return err
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
