//go:build linux

package container

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

const processRlimitNOFILEEnv = "MINICONTAINER_PROCESS_RLIMIT_NOFILE"

// applyProcessRlimitRuntimeMarker runs in the re-executed container-init
// generation. OCI process rlimits belong to that container process; the
// internal PID 1 supervisor and its payload therefore inherit the same kernel
// limits. The marker is removed before the payload environment is constructed.
func init() {
	if os.Getenv(sentinelEnvKey) != "1" {
		return
	}
	raw, ok := os.LookupEnv(processRlimitNOFILEEnv)
	if !ok {
		return
	}
	if err := os.Unsetenv(processRlimitNOFILEEnv); err != nil {
		failProcessRlimitInit("clear runtime marker: %v", err)
	}
	parts := strings.Split(raw, ":")
	if len(parts) != 2 {
		failProcessRlimitInit("invalid runtime marker %q", raw)
	}
	soft, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		failProcessRlimitInit("invalid soft limit in runtime marker %q: %v", raw, err)
	}
	hard, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		failProcessRlimitInit("invalid hard limit in runtime marker %q: %v", raw, err)
	}
	if soft > hard {
		failProcessRlimitInit("soft limit %d exceeds hard limit %d", soft, hard)
	}
	if err := unix.Setrlimit(unix.RLIMIT_NOFILE, &unix.Rlimit{Cur: soft, Max: hard}); err != nil {
		failProcessRlimitInit("set RLIMIT_NOFILE %d:%d: %v", soft, hard, err)
	}
}

func failProcessRlimitInit(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "container init: process rlimit: "+format+"\n", args...)
	os.Exit(126)
}
