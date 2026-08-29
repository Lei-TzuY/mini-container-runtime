package dns

import (
	"fmt"
	"strings"
)

// CleanupStoppedLegacyRegistrarHostRegistration retires only generation-unaware
// registrar-owned DNS records for a stopped container. It is migration-only:
// modern generation-aware records and pre-ownership ownerless records are never
// consumed here. Unlike the older compatibility path, recovery does not need to
// resolve the identity of the process performing cleanup; each legacy owner is
// judged solely by its own liveness.
func CleanupStoppedLegacyRegistrarHostRegistration(networkName, containerID string) error {
	return cleanupStoppedLegacyRegistrarHostRegistrationWith(networkName, containerID, hostEntryOwnerActive)
}

func cleanupStoppedLegacyRegistrarHostRegistrationWith(
	networkName, containerID string,
	ownerActive hostEntryActiveProbe,
) error {
	if err := validateNetworkName(networkName); err != nil {
		return err
	}
	if strings.TrimSpace(containerID) == "" {
		return fmt.Errorf("container ID cannot be empty")
	}
	if ownerActive == nil {
		return fmt.Errorf("DNS ownership activity probe is nil")
	}

	return withStoppedDNSRegistry(networkName, func(entries []HostEntry) ([]HostEntry, bool, error) {
		updated := make([]HostEntry, 0, len(entries))
		removed := false
		for _, entry := range entries {
			if entry.ContainerID != containerID || entry.GenerationAware {
				updated = append(updated, entry)
				continue
			}
			if entry.OwnerPID == 0 && entry.OwnerStartTime == 0 {
				updated = append(updated, entry)
				continue
			}

			active, err := ownerActive(entry)
			if err != nil {
				return nil, false, fmt.Errorf("resolve legacy DNS ownership before stopped cleanup for container %s: %w", containerID, err)
			}
			if active {
				updated = append(updated, entry)
				continue
			}
			removed = true
		}
		return updated, removed, nil
	})
}
