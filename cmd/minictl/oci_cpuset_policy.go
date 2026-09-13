package main

import (
	"fmt"
	"strings"
)

// extractOCICPUSetPolicy removes parent-only OCI cpuset markers from the
// payload environment and returns the durable cgroup policy they encode.
func extractOCICPUSetPolicy(env []string) (clean []string, cpus, mems string, err error) {
	clean = make([]string, 0, len(env))
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		switch key {
		case processCPUSetCPUsEnv:
			if !ok || value == "" {
				return nil, "", "", fmt.Errorf("invalid internal cpuset CPUs marker")
			}
			if cpus != "" {
				return nil, "", "", fmt.Errorf("duplicate internal cpuset CPUs marker")
			}
			cpus = value
		case processCPUSetMemsEnv:
			if !ok || value == "" {
				return nil, "", "", fmt.Errorf("invalid internal cpuset mems marker")
			}
			if mems != "" {
				return nil, "", "", fmt.Errorf("duplicate internal cpuset mems marker")
			}
			mems = value
		default:
			clean = append(clean, entry)
		}
	}
	return clean, cpus, mems, nil
}
