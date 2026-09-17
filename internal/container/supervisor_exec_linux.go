//go:build linux

package container

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
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
	fd := file.Fd()
	flags, err := unix.FcntlInt(fd, unix.F_GETFD, 0)
	if err != nil {
		return "", fmt.Errorf("inspect init supervisor executable fd: %w", err)
	}
	// syscall.Exec resolves /proc/self/fd/<n> during exec. Keep the pinned
	// descriptor alive across that boundary; the supervisor closes it
	// immediately after startup, before it forks the payload.
	if _, err := unix.FcntlInt(fd, unix.F_SETFD, flags &^ unix.FD_CLOEXEC); err != nil {
		return "", fmt.Errorf("preserve init supervisor executable fd across exec: %w", err)
	}
	return fmt.Sprintf("/proc/self/fd/%d", fd), nil
}
