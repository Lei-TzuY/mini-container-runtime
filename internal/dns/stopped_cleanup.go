package dns

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type hostEntryActiveProbe func(HostEntry) (bool, error)

// CleanupStoppedHostRegistration removes service-discovery state after an
// authoritative stopped-container finalization. Registrations owned by this
// exact registrar generation can be consumed directly. A registration owned by
// another generation is removed only when that ownership is provably stale;
// live replacement/adopted generations and legacy entries without ownership
// proof are retained.
func CleanupStoppedHostRegistration(networkName, containerID string) error {
	owner, err := currentRegistrarIdentity()
	if err != nil {
		return err
	}
	return cleanupStoppedHostRegistrationWith(networkName, containerID, owner, hostEntryOwnerActive)
}

func cleanupStoppedHostRegistrationWith(
	networkName, containerID string,
	currentOwner registrarIdentity,
	ownerActive hostEntryActiveProbe,
) error {
	if err := validateNetworkName(networkName); err != nil {
		return err
	}
	if strings.TrimSpace(containerID) == "" {
		return fmt.Errorf("container ID cannot be empty")
	}
	if currentOwner.PID <= 0 || currentOwner.StartTime == 0 {
		return fmt.Errorf("invalid current DNS registrar process identity %d/%d", currentOwner.PID, currentOwner.StartTime)
	}
	if ownerActive == nil {
		return fmt.Errorf("DNS ownership activity probe is nil")
	}

	// Stopped finalization runs for containers that may never have used bridge
	// networking. Absence must remain a side-effect-free no-op.
	dir := DefaultDNSDir()
	info, err := os.Lstat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect DNS registry directory %q: %w", dir, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("DNS registry path %q must be a real directory", dir)
	}

	dnsMu.Lock()
	defer dnsMu.Unlock()
	return withDNSNetworkLock(dir, networkName, func() error {
		netFile := filepath.Join(dir, networkName+".json")
		entries, exists, err := loadEntriesChecked(netFile, networkName)
		if err != nil || !exists {
			return err
		}

		updated := make([]HostEntry, 0, len(entries))
		removed := false
		for _, entry := range entries {
			if entry.ContainerID != containerID {
				updated = append(updated, entry)
				continue
			}

			// Legacy entries have no generation proof. Never guess that they are
			// stale merely because a modern finalizer is running.
			if entry.OwnerPID == 0 && entry.OwnerStartTime == 0 {
				updated = append(updated, entry)
				continue
			}

			if entry.OwnerPID == currentOwner.PID && entry.OwnerStartTime == currentOwner.StartTime {
				removed = true
				continue
			}

			active, err := ownerActive(entry)
			if err != nil {
				return fmt.Errorf("resolve DNS ownership before stopped cleanup for container %s: %w", containerID, err)
			}
			if active {
				updated = append(updated, entry)
				continue
			}
			removed = true
		}
		if !removed {
			return nil
		}
		return saveEntriesAtomic(dir, netFile, networkName, updated)
	})
}
