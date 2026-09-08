//go:build linux

package container

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"minicontainer/internal/ns"
)

const (
	noNewPrivilegesEnv = "MINICONTAINER_NO_NEW_PRIVILEGES"
	processUIDEnv       = "MINICONTAINER_PROCESS_UID"
	processGIDEnv       = "MINICONTAINER_PROCESS_GID"
	processGroupsEnv    = "MINICONTAINER_PROCESS_GROUPS"
)

var securityPolicyRunMu sync.Mutex

// RunWithSecurityPolicy launches a container while propagating parent-only
// Linux security/isolation policy into the re-executed init generation without
// leaking runtime markers into the payload environment.
func RunWithSecurityPolicy(cfg Config) error {
	if cfg.ProcessUser != nil && cfg.UserNS {
		if cfg.ProcessUser.UID != 0 || cfg.ProcessUser.GID != 0 {
			return fmt.Errorf("process user %d:%d cannot be represented by one-entry user namespace mapping", cfg.ProcessUser.UID, cfg.ProcessUser.GID)
		}
		if len(cfg.ProcessUser.Groups) != 0 {
			return fmt.Errorf("supplementary process groups cannot be represented while setgroups is disabled in the user namespace")
		}
	}
	if cfg.ProcessUser != nil {
		for _, entry := range cfg.Env {
			key := entry
			if i := strings.IndexByte(key, '='); i >= 0 {
				key = key[:i]
			}
			if key == processUIDEnv || key == processGIDEnv || key == processGroupsEnv {
				return fmt.Errorf("payload environment key %q conflicts with internal process user policy", key)
			}
		}
	}
	if !cfg.NoNewPrivileges && !cfg.CgroupNS && cfg.ProcessUser == nil {
		return Run(cfg)
	}
	securityPolicyRunMu.Lock()
	defer securityPolicyRunMu.Unlock()

	restore := make([]func(), 0, 5)
	defer func() {
		for i := len(restore) - 1; i >= 0; i-- {
			restore[i]()
		}
	}()
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
		groups := make([]string, 0, len(cfg.ProcessUser.Groups))
		for _, gid := range cfg.ProcessUser.Groups {
			groups = append(groups, strconv.FormatUint(uint64(gid), 10))
		}
		if err := setMarker(processGroupsEnv, strings.Join(groups, ",")); err != nil {
			return fmt.Errorf("set process groups runtime marker: %w", err)
		}
	}
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
