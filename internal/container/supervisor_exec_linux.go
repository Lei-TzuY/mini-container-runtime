//go:build linux

package container

import (
	"fmt"
	"os"
)

// InitSupervisorArg is the internal argv marker used when the container's PID 1
// re-executes the runtime as the payload supervisor.
const InitSupervisorArg = "__minicontainer-init-supervisor"

func pinInitSupervisorExecutable(command []string) (*os.File, error) {
	if len(command) < 2 || command[0] != "/proc/self/exe" || command[1] != InitSupervisorArg {
		return nil, nil
	}
	file, err := os.Open("/proc/self/exe")
	if err != nil {
		return nil, fmt.Errorf("pin init supervisor executable: %w", err)
	}
	return file, nil
}

func initSupervisorExecutablePath(file *os.File) (string, error) {
	if file == nil {
		return "", fmt.Errorf("init supervisor executable is nil")
	}
	return fmt.Sprintf("/proc/self/fd/%d", file.Fd()), nil
}
