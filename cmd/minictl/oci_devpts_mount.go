package main

import (
	"fmt"
	"runtime"
)

func validateOCIDevptsMount(destination, source string, options []string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("OCI devpts mounts require linux")
	}
	if source != "" && source != "devpts" {
		return fmt.Errorf("OCI devpts mount source %q must be empty or devpts", source)
	}
	if destination != "/dev/pts" {
		return fmt.Errorf("OCI devpts mount destination %q is not representable; runtime devpts is mounted at /dev/pts", destination)
	}

	want := map[string]struct{}{
		"nosuid":        {},
		"noexec":        {},
		"newinstance":   {},
		"ptmxmode=0666": {},
		"mode=0666":     {},
	}
	seen := make(map[string]struct{}, len(options))
	for _, option := range options {
		if _, duplicate := seen[option]; duplicate {
			return fmt.Errorf("duplicate OCI devpts mount option %q", option)
		}
		seen[option] = struct{}{}
		if _, ok := want[option]; !ok {
			return fmt.Errorf("unsupported OCI mount type \"devpts\": option %q is not representable by the runtime's fixed /dev/pts mount semantics", option)
		}
	}
	if len(seen) != len(want) {
		return fmt.Errorf("unsupported OCI mount type \"devpts\": options must exactly represent nosuid/noexec/newinstance/ptmxmode=0666/mode=0666 runtime semantics")
	}
	for option := range want {
		if _, ok := seen[option]; !ok {
			return fmt.Errorf("unsupported OCI mount type \"devpts\": missing required option %q", option)
		}
	}
	return nil
}
