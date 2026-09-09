package dns

import (
	"fmt"
	"strings"
)

// BindHostRegistrationGeneration durably upgrades a registrar-owned DNS entry
// to exact child-generation ownership after bridge admission succeeds. Runtime
// admission reservations become visible to peers in the same atomic registry
// update that binds the exact child generation.
func BindHostRegistrationGeneration(networkName, containerID string, pid int, pidStartTime uint64) error {
	return bindHostRegistrationGeneration(networkName, containerID, pid, pidStartTime, "")
}

// BindHostRegistrationGenerationAddress atomically publishes the exact bridge
// address selected for a child generation while binding durable generation
// ownership. This closes the gap between admission-time DNS reservation and
// generation-scoped IPAM: peers never observe the placeholder admission address
// once the generation becomes visible.
func BindHostRegistrationGenerationAddress(networkName, containerID string, pid int, pidStartTime uint64, ipAddr string) error {
	canonicalIP, err := canonicalIPAddress(ipAddr)
	if err != nil {
		return err
	}
	return bindHostRegistrationGeneration(networkName, containerID, pid, pidStartTime, canonicalIP)
}

func bindHostRegistrationGeneration(networkName, containerID string, pid int, pidStartTime uint64, publishedIP string) error {
	if err := validateNetworkName(networkName); err != nil {
		return err
	}
	if strings.TrimSpace(containerID) == "" {
		return fmt.Errorf("container ID cannot be empty")
	}
	if pid <= 0 || pidStartTime == 0 {
		return fmt.Errorf("invalid DNS child process identity %d/%d", pid, pidStartTime)
	}
	owner, err := currentRegistrarIdentity()
	if err != nil {
		return err
	}

	dnsMu.Lock()
	defer dnsMu.Unlock()
	dir, err := ensureDNSDir()
	if err != nil {
		return err
	}
	return withDNSNetworkLock(dir, networkName, func(dirFD int) error {
		netName := networkName + ".json"
		entries, exists, err := loadEntriesCheckedAt(dirFD, netName, networkName)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("DNS registration for container %q does not exist", containerID)
		}
		for i, entry := range entries {
			if entry.ContainerID != containerID {
				continue
			}
			if entry.OwnerPID != owner.PID || entry.OwnerStartTime != owner.StartTime {
				return fmt.Errorf(
					"DNS registration for container %q is owned by registrar %d/%d, not current registrar %d/%d",
					containerID,
					entry.OwnerPID,
					entry.OwnerStartTime,
					owner.PID,
					owner.StartTime,
				)
			}
			if !entry.GenerationAware {
				return fmt.Errorf("DNS registration for container %q is not generation-aware", containerID)
			}
			if entry.GenerationPID == pid && entry.GenerationStartTime == pidStartTime && !entry.AdmissionPending {
				if publishedIP == "" || entry.IP == publishedIP {
					return nil
				}
				return fmt.Errorf(
					"DNS registration for container %q generation %d/%d is already published at %s, not %s",
					containerID,
					pid,
					pidStartTime,
					entry.IP,
					publishedIP,
				)
			}
			if (entry.GenerationPID != 0 || entry.GenerationStartTime != 0) &&
				(entry.GenerationPID != pid || entry.GenerationStartTime != pidStartTime) {
				return fmt.Errorf(
					"DNS registration for container %q is already bound to child generation %d/%d",
					containerID,
					entry.GenerationPID,
					entry.GenerationStartTime,
				)
			}
			updated := append([]HostEntry(nil), entries...)
			if publishedIP != "" {
				updated[i].IP = publishedIP
			}
			updated[i].GenerationPID = pid
			updated[i].GenerationStartTime = pidStartTime
			updated[i].AdmissionPending = false
			return saveEntriesAtomicAt(dirFD, netName, networkName, updated)
		}
		return fmt.Errorf("DNS registration for container %q does not exist", containerID)
	})
}
