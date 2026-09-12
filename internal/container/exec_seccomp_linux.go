//go:build linux

package container

import (
	"fmt"
	"os"
	"strconv"
)

// Exec helpers are spawned by the host runtime, so they do not inherit the
// container init's seccomp filter. Re-apply the durable syscall policy before
// ExecInit joins namespaces and starts the exec payload.
func init() {
	if os.Getenv(execSentinelKey) != "1" || len(os.Args) < 4 || os.Args[1] != "exec" {
		return
	}
	pid, err := strconv.Atoi(os.Args[2])
	if err != nil || pid <= 0 {
		return
	}
	if err := applyPersistedExecSeccomp(pid, os.Args[3]); err != nil {
		fmt.Fprintf(os.Stderr, "exec seccomp policy: %v\n", err)
		os.Exit(126)
	}
}

func applyPersistedExecSeccomp(containerPID int, rootFS string) error {
	spec, err := persistedExecSpec(containerPID, rootFS, "seccomp policy")
	if err != nil {
		return err
	}
	return applyExecSeccomp(spec.Seccomp)
}

func applyExecSeccomp(enabled bool) error {
	if !enabled {
		return nil
	}
	return applySeccomp(false)
}
