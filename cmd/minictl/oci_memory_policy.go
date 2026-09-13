package main

import (
	"fmt"
	"strconv"
	"strings"
)

const processMemoryHighEnv = "MINICONTAINER_CGROUP_MEMORY_HIGH"

// extractOCIMemoryPolicy removes the parent-only memory.high marker from the
// payload environment and returns the durable cgroup v2 soft limit it encodes.
func extractOCIMemoryPolicy(env []string) (clean []string, memoryHigh int64, err error) {
	clean = make([]string, 0, len(env))
	seen := false
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if key != processMemoryHighEnv {
			clean = append(clean, entry)
			continue
		}
		if seen {
			return nil, 0, fmt.Errorf("duplicate internal memory high marker")
		}
		seen = true
		if !ok || value == "" {
			return nil, 0, fmt.Errorf("invalid internal memory high marker")
		}
		memoryHigh, err = strconv.ParseInt(value, 10, 64)
		if err != nil || memoryHigh <= 0 {
			return nil, 0, fmt.Errorf("invalid internal memory high marker %q", value)
		}
	}
	return clean, memoryHigh, nil
}
