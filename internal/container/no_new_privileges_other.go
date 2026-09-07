//go:build !linux

package container

import "fmt"

// RunWithSecurityPolicy preserves platform boundaries for Linux-only process
// security and namespace semantics instead of silently weakening policy.
func RunWithSecurityPolicy(cfg Config) error {
	if cfg.NoNewPrivileges {
		return fmt.Errorf("no-new-privileges requires linux")
	}
	if cfg.CgroupNS {
		return fmt.Errorf("cgroup namespace requires linux")
	}
	if cfg.ProcessUser != nil {
		return fmt.Errorf("process user requires linux")
	}
	return Run(cfg)
}
