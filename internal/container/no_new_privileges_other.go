//go:build !linux

package container

import "fmt"

// RunWithSecurityPolicy preserves the platform boundary for Linux-only
// no-new-privileges semantics instead of silently weakening requested policy.
func RunWithSecurityPolicy(cfg Config) error {
	if cfg.NoNewPrivileges {
		return fmt.Errorf("no-new-privileges requires linux")
	}
	return Run(cfg)
}
