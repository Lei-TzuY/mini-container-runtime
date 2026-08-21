// Package imagestore provides OCI image configuration inspection utilities.
// This file implements an exposed port conflict detector that compares
// two or more image configs and identifies overlapping port declarations.

package imagestore

import (
	"fmt"
	"sort"
	"strings"
)

// PortConflict describes a port that is declared in multiple image configs.
type PortConflict struct {
	Port   string   // e.g. "80/tcp"
	Images []string // names/indices of images declaring this port
}

// DetectPortConflicts compares exposed ports across multiple image configs
// and returns any ports declared in more than one image.
func DetectPortConflicts(configs map[string][]byte) ([]PortConflict, error) {
	portOwners := make(map[string][]string)

	for name, configJSON := range configs {
		summary, err := ExtractExposedPorts(configJSON)
		if err != nil {
			return nil, fmt.Errorf("image %q: %w", name, err)
		}
		for _, entry := range summary.Ports {
			key := fmt.Sprintf("%d/%s", entry.Port, entry.Protocol)
			portOwners[key] = append(portOwners[key], name)
		}
	}

	var conflicts []PortConflict
	for port, owners := range portOwners {
		if len(owners) > 1 {
			sort.Strings(owners)
			conflicts = append(conflicts, PortConflict{Port: port, Images: owners})
		}
	}
	sort.Slice(conflicts, func(i, j int) bool {
		return conflicts[i].Port < conflicts[j].Port
	})
	return conflicts, nil
}

// FormatPortConflicts returns a human-readable conflict report.
func FormatPortConflicts(conflicts []PortConflict) string {
	if len(conflicts) == 0 {
		return "Port Conflicts: (none detected)"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Port Conflicts: %d overlapping ports\n", len(conflicts)))
	for _, c := range conflicts {
		sb.WriteString(fmt.Sprintf("  %s -> %s\n", c.Port, strings.Join(c.Images, ", ")))
	}
	return strings.TrimRight(sb.String(), "\n")
}
