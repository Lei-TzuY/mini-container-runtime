package main

import (
	"fmt"
	"runtime"
)

func validateOCIMqueueMount(destination, source string, options []string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("OCI mqueue mounts require linux")
	}
	if source != "" && source != "mqueue" {
		return fmt.Errorf("OCI mqueue mount source %q must be empty or mqueue", source)
	}
	if destination != "/dev/mqueue" {
		return fmt.Errorf("OCI mqueue mount destination %q is not representable; runtime mqueue is mounted at /dev/mqueue", destination)
	}

	want := map[string]struct{}{
		"nosuid": {},
		"noexec": {},
		"nodev":  {},
	}
	seen := make(map[string]struct{}, len(options))
	for _, option := range options {
		if _, duplicate := seen[option]; duplicate {
			return fmt.Errorf("duplicate OCI mqueue mount option %q", option)
		}
		seen[option] = struct{}{}
		if _, ok := want[option]; !ok {
			return fmt.Errorf("unsupported OCI mount type \"mqueue\": option %q is not representable by the runtime's fixed /dev/mqueue mount semantics", option)
		}
	}
	if len(seen) != len(want) {
		return fmt.Errorf("unsupported OCI mount type \"mqueue\": options must exactly represent nosuid/noexec/nodev runtime semantics")
	}
	for option := range want {
		if _, ok := seen[option]; !ok {
			return fmt.Errorf("unsupported OCI mount type \"mqueue\": missing required option %q", option)
		}
	}
	return nil
}
