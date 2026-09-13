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

// OCI process.ioPriority belongs to the workload process tree. Apply it in
// re-executed container-init and exec generations; descendants inherit the I/O
// priority across fork/exec. The marker is removed before payload execution.
func init() {
	if os.Getenv(sentinelEnvKey) != "1" && os.Getenv(execSentinelKey) != "1" {
		return
	}
	raw, ok := os.LookupEnv(processIOPriorityEnv)
	if !ok {
		return
	}
	if err := os.Unsetenv(processIOPriorityEnv); err != nil {
		failProcessIOPriorityInit("clear runtime marker: %v", err)
	}

	className, priorityText, ok := strings.Cut(strings.TrimSpace(raw), ":")
	if !ok || className == "" || priorityText == "" || strings.Contains(priorityText, ":") {
		failProcessIOPriorityInit("invalid runtime marker %q", raw)
	}
	priority, err := strconv.Atoi(priorityText)
	if err != nil || priority < 0 || priority > 7 {
		failProcessIOPriorityInit("invalid priority %q", priorityText)
	}
	class, ok := ioPriorityClassValue(className)
	if !ok {
		failProcessIOPriorityInit("invalid class %q", className)
	}

	value := (class << ioPriorityClassShift) | priority
	if _, _, errno := unix.Syscall(unix.SYS_IOPRIO_SET, uintptr(ioPriorityWhoProcess), 0, uintptr(value)); errno != 0 {
		failProcessIOPriorityInit("set %s:%d: %v", className, priority, errno)
	}
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

func failProcessIOPriorityInit(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "container init: process io priority: "+format+"\n", args...)
	os.Exit(126)
}
