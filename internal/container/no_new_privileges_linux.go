//go:build linux

package container

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"syscall"

	"minicontainer/internal/ns"
)

const (
	noNewPrivilegesEnv = "MINICONTAINER_NO_NEW_PRIVILEGES"
	processUIDEnv       = "MINICONTAINER_PROCESS_UID"
	processGIDEnv       = "MINICONTAINER_PROCESS_GID"
)

var securityPolicyRunMu sync.Mutex

// RunWithSecurityPolicy launches a container while propagating parent-only
// Linux security/isolation policy into the re-executed init generation without
// leaking runtime markers into the payload environment.
func RunWithSecurityPolicy(cfg Config) error {
	if cfg.ProcessUser != nil && cfg.UserNS && (cfg.ProcessUser.UID != 0 || cfg.ProcessUser.GID != 0) {
		return fmt.Errorf("process user %d:%d cannot be represented by one-entry user namespace mapping", cfg.ProcessUser.UID, cfg.ProcessUser.GID)
	}
	if !cfg.NoNewPrivileges && !cfg.CgroupNS && cfg.ProcessUser == nil {
		return Run(cfg)
	}
	securityPolicyRunMu.Lock()
	defer securityPolicyRunMu.Unlock()

	restore := make([]func(), 0, 4)
	setMarker := func(key, value string) error {
		old, hadOld := os.LookupEnv(key)
		if err := os.Setenv(key, value); err != nil {
			return err
		}
		restore = append(restore, func() {
			if hadOld {
				_ = os.Setenv(key, old)
			} else {
				_ = os.Unsetenv(key)
			}
		})
		return nil
	}
	if cfg.NoNewPrivileges {
		if err := setMarker(noNewPrivilegesEnv, "1"); err != nil {
			return fmt.Errorf("set no-new-privileges runtime marker: %w", err)
		}
	}
	if cfg.CgroupNS {
		if err := setMarker(ns.CgroupNamespaceEnv, "1"); err != nil {
			for i := len(restore) - 1; i >= 0; i-- {
				restore[i]()
			}
			return fmt.Errorf("set cgroup namespace runtime marker: %w", err)
		}
	}
	if cfg.ProcessUser != nil {
		if err := setMarker(processUIDEnv, strconv.FormatUint(uint64(cfg.ProcessUser.UID), 10)); err != nil {
			return fmt.Errorf("set process uid runtime marker: %w", err)
		}
		if err := setMarker(processGIDEnv, strconv.FormatUint(uint64(cfg.ProcessUser.GID), 10)); err != nil {
			return fmt.Errorf("set process gid runtime marker: %w", err)
		}
	}
	defer func() {
		for i := len(restore) - 1; i >= 0; i-- {
			restore[i]()
		}
	}()
	return Run(cfg)
}

func init() {
	if os.Getenv(sentinelEnvKey) != "1" {
		return
	}
	// The cgroup-namespace marker is parent-only: clone(2) already consumed it.
	_ = os.Unsetenv(ns.CgroupNamespaceEnv)
	if os.Getenv(noNewPrivilegesEnv) != "1" {
		return
	}
	if _, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL, prSetNoNewPrivs, 1, 0); errno != 0 {
		fmt.Fprintf(os.Stderr, "container init: prctl(PR_SET_NO_NEW_PRIVS): %v\n", errno)
		os.Exit(126)
	}
	_ = os.Unsetenv(noNewPrivilegesEnv)
}
