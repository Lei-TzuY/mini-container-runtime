//go:build linux

package container

import (
	"fmt"
	"os"
	"strconv"
	"syscall"
)

// exec helpers are spawned by the host runtime, so they do not inherit the
// container init's PR_SET_NO_NEW_PRIVS bit. Re-apply the durable process policy
// before ExecInit joins namespaces and starts the exec payload.
func init() {
	if os.Getenv(execSentinelKey) != "1" || len(os.Args) < 4 || os.Args[1] != "exec" {
		return
	}
	pid, err := strconv.Atoi(os.Args[2])
	if err != nil || pid <= 0 {
		return
	}
	if err := applyPersistedExecNoNewPrivileges(pid, os.Args[3]); err != nil {
		fmt.Fprintf(os.Stderr, "exec security policy: %v\n", err)
		os.Exit(126)
	}
}

func applyPersistedExecNoNewPrivileges(containerPID int, rootFS string) error {
	spec, err := persistedExecSpec(containerPID, rootFS, "no-new-privileges policy")
	if err != nil {
		return err
	}
	return applyExecNoNewPrivileges(spec.NoNewPrivileges)
}

func applyExecNoNewPrivileges(enabled bool) error {
	if !enabled {
		return nil
	}
	if _, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL, prSetNoNewPrivs, 1, 0); errno != 0 {
		return fmt.Errorf("prctl(PR_SET_NO_NEW_PRIVS): %w", errno)
	}
	return nil
}
