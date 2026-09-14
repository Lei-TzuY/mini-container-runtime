package main

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	processMemoryHighEnv = "MINICONTAINER_CGROUP_MEMORY_HIGH"
	processMemorySwapEnv = "MINICONTAINER_CGROUP_MEMORY_SWAP"
)

// extractOCIMemoryPolicy removes parent-only memory policy markers from the
// payload environment and returns the durable cgroup v2 limits they encode.
func extractOCIMemoryPolicy(env []string) (clean []string, memoryHigh, memorySwap int64, err error) {
	clean = make([]string, 0, len(env))
	seenHigh := false
	seenSwap := false
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		switch key {
		case processMemoryHighEnv:
			if seenHigh {
				return nil, 0, 0, fmt.Errorf("duplicate internal memory high marker")
			}
			seenHigh = true
			if !ok || value == "" {
				return nil, 0, 0, fmt.Errorf("invalid internal memory high marker")
			}
			memoryHigh, err = strconv.ParseInt(value, 10, 64)
			if err != nil || memoryHigh <= 0 {
				return nil, 0, 0, fmt.Errorf("invalid internal memory high marker %q", value)
			}
		case processMemorySwapEnv:
			if seenSwap {
				return nil, 0, 0, fmt.Errorf("duplicate internal memory swap marker")
			}
			seenSwap = true
			if !ok || value == "" {
				return nil, 0, 0, fmt.Errorf("invalid internal memory swap marker")
			}
			memorySwap, err = strconv.ParseInt(value, 10, 64)
			if err != nil || (memorySwap != -1 && memorySwap <= 0) {
				return nil, 0, 0, fmt.Errorf("invalid internal memory swap marker %q", value)
			}
		default:
			clean = append(clean, entry)
		}
	}
	return clean, memoryHigh, memorySwap, nil
}
