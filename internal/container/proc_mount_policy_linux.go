//go:build linux

package container

import (
	"fmt"
	"strings"
	"syscall"
)

// RuntimeProcMountOptionsEnvKey is reserved for carrying OCI /proc mount policy
// across the runtime's self re-exec. ContainerInit removes it before payload exec.
const RuntimeProcMountOptionsEnvKey = "MINICONTAINER_INTERNAL_PROC_MOUNT_OPTIONS"

func extractProcMountPolicy(env []string) ([]string, uintptr, error) {
	prefix := RuntimeProcMountOptionsEnvKey + "="
	clean := make([]string, 0, len(env))
	var raw string
	found := false
	for _, entry := range env {
		if !strings.HasPrefix(entry, prefix) {
			clean = append(clean, entry)
			continue
		}
		if found {
			return nil, 0, fmt.Errorf("duplicate internal proc mount policy")
		}
		found = true
		raw = strings.TrimPrefix(entry, prefix)
	}
	if !found {
		return clean, 0, nil
	}
	if raw == "" {
		return nil, 0, fmt.Errorf("internal proc mount policy is empty")
	}
	flags, err := procMountFlags(strings.Split(raw, ","))
	if err != nil {
		return nil, 0, err
	}
	return clean, flags, nil
}

func procMountFlags(options []string) (uintptr, error) {
	flags := uintptr(0)
	seen := make(map[string]struct{}, len(options))
	for _, option := range options {
		if option == "" || strings.TrimSpace(option) != option {
			return 0, fmt.Errorf("invalid proc mount option %q", option)
		}
		if _, duplicate := seen[option]; duplicate {
			return 0, fmt.Errorf("duplicate proc mount option %q", option)
		}
		seen[option] = struct{}{}
		switch option {
		case "ro":
			flags |= syscall.MS_RDONLY
		case "rw":
			flags &^= syscall.MS_RDONLY
		case "nosuid":
			flags |= syscall.MS_NOSUID
		case "suid":
			flags &^= syscall.MS_NOSUID
		case "noexec":
			flags |= syscall.MS_NOEXEC
		case "exec":
			flags &^= syscall.MS_NOEXEC
		case "nodev":
			flags |= syscall.MS_NODEV
		case "dev":
			flags &^= syscall.MS_NODEV
		default:
			return 0, fmt.Errorf("unsupported proc mount option %q", option)
		}
	}
	return flags, nil
}
