//go:build linux

package container

import (
	"fmt"
	"os"
	"sync"
	"syscall"
)

const noNewPrivilegesEnv = "MINICONTAINER_NO_NEW_PRIVILEGES"

var noNewPrivilegesRunMu sync.Mutex

// RunWithSecurityPolicy launches a container while propagating parent-only
// security policy into the re-executed init generation without leaking the
// runtime marker into the payload environment.
func RunWithSecurityPolicy(cfg Config) error {
	if !cfg.NoNewPrivileges {
		return Run(cfg)
	}
	noNewPrivilegesRunMu.Lock()
	defer noNewPrivilegesRunMu.Unlock()

	old, hadOld := os.LookupEnv(noNewPrivilegesEnv)
	if err := os.Setenv(noNewPrivilegesEnv, "1"); err != nil {
		return fmt.Errorf("set no-new-privileges runtime marker: %w", err)
	}
	defer func() {
		if hadOld {
			_ = os.Setenv(noNewPrivilegesEnv, old)
		} else {
			_ = os.Unsetenv(noNewPrivilegesEnv)
		}
	}()
	return Run(cfg)
}

func init() {
	if os.Getenv(sentinelEnvKey) != "1" || os.Getenv(noNewPrivilegesEnv) != "1" {
		return
	}
	if _, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL, prSetNoNewPrivs, 1, 0); errno != 0 {
		fmt.Fprintf(os.Stderr, "container init: prctl(PR_SET_NO_NEW_PRIVS): %v\n", errno)
		os.Exit(126)
	}
	_ = os.Unsetenv(noNewPrivilegesEnv)
}
