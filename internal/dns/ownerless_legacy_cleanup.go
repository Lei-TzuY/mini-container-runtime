package dns

import (
	"fmt"
	"strings"
)

// CleanupStoppedOwnerlessLegacyHostRegistration removes only pre-ownership DNS
// records for one container. Current runtimes never create this entry class, so
// callers that have already proven an authoritative stopped lifecycle revision
// may synchronously retire migration debris without gaining authority over
// registrar-owned or generation-aware registrations created by a restart.
func CleanupStoppedOwnerlessLegacyHostRegistration(networkName, containerID string) error {
	if err := validateNetworkName(networkName); err != nil {
		return err
	}
	if strings.TrimSpace(containerID) == "" {
		return fmt.Errorf("container ID cannot be empty")
	}

	return withStoppedDNSRegistry(networkName, func(entries []HostEntry) ([]HostEntry, bool, error) {
		updated := make([]HostEntry, 0, len(entries))
		removed := false
		for _, entry := range entries {
			if entry.ContainerID == containerID &&
				!entry.GenerationAware &&
				entry.OwnerPID == 0 &&
				entry.OwnerStartTime == 0 {
				removed = true
				continue
			}
			updated = append(updated, entry)
		}
		return updated, removed, nil
	})
}
