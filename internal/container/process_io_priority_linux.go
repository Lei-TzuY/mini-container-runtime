//go:build linux

package container

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

const processIOPriorityEnv = "MINICONTAINER_PROCESS_IO_PRIORITY"

const (
	ioPriorityWhoProcess = 1
	ioPriorityClassShift = 13
	ioPriorityClassRT    = 1
	ioPriorityClassBE    = 2
	ioPriorityClassIdle  = 3
)

// applyProcessIOPriorityPolicy consumes the runtime marker on the OS thread
// that will fork the workload, so descendants inherit the requested policy.
func applyProcessIOPriorityPolicy() error {
	raw, ok := os.LookupEnv(processIOPriorityEnv)
	if !ok {
		return nil
	}
	if err := os.Unsetenv(processIOPriorityEnv); err != nil {
		return fmt.Errorf("clear runtime marker: %w", err)
	}

	className, priorityText, ok := strings.Cut(strings.TrimSpace(raw), ":")
	if !ok || className == "" || priorityText == "" || strings.Contains(priorityText, ":") {
		return fmt.Errorf("invalid runtime marker %q", raw)
	}
	priority, err := strconv.Atoi(priorityText)
	if err != nil || priority < 0 || priority > 7 {
		return fmt.Errorf("invalid priority %q", priorityText)
	}
	class, ok := ioPriorityClassValue(className)
	if !ok {
		return fmt.Errorf("invalid class %q", className)
	}

	value := (class << ioPriorityClassShift) | priority
	if _, _, errno := unix.Syscall(unix.SYS_IOPRIO_SET, uintptr(ioPriorityWhoProcess), 0, uintptr(value)); errno != 0 {
		return fmt.Errorf("set %s:%d: %w", className, priority, errno)
	}
	return nil
}

func ioPriorityClassValue(name string) (int, bool) {
	switch name {
	case "IOPRIO_CLASS_RT":
		return ioPriorityClassRT, true
	case "IOPRIO_CLASS_BE":
		return ioPriorityClassBE, true
	case "IOPRIO_CLASS_IDLE":
		return ioPriorityClassIdle, true
	default:
		return 0, false
	}
}

