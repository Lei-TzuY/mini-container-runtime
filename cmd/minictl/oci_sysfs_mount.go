package main

import (
	"fmt"
	"runtime"
)

func validateOCISysfsMount(destination, source string, options []string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("OCI sysfs mounts require linux")
	}
	if source != "" && source != "sysfs" {
		return fmt.Errorf("OCI sysfs mount source %q must be empty or sysfs", source)
	}
	if destination != "/sys" {
		return fmt.Errorf("OCI sysfs mount destination %q is not representable; runtime sysfs is mounted at /sys", destination)
	}

	want := map[string]struct{}{
		"ro":     {},
		"nosuid": {},
		"noexec": {},
		"nodev":  {},
	}
	seen := make(map[string]struct{}, len(options))
	for _, option := range options {
		if _, duplicate := seen[option]; duplicate {
			return fmt.Errorf("duplicate OCI sysfs mount option %q", option)
		}
		seen[option] = struct{}{}
		if _, ok := want[option]; !ok {
			return fmt.Errorf("unsupported OCI mount type \"sysfs\": option %q is not representable by the runtime's fixed /sys mount semantics", option)
		}
	}
	if len(seen) != len(want) {
		return fmt.Errorf("unsupported OCI mount type \"sysfs\": options must exactly represent read-only nosuid/noexec/nodev runtime semantics")
	}
	for option := range want {
		if _, ok := seen[option]; !ok {
			return fmt.Errorf("unsupported OCI mount type \"sysfs\": missing required option %q", option)
		}
	}
	return nil
}
