package dns

import (
	"fmt"
	"strings"
	"sync"
)

type generationAddressKey struct {
	networkName string
	containerID string
}

type generationAddressReservation struct {
	token string
	ip    string
}

var (
	generationAddressMu sync.Mutex
	generationAddresses = make(map[generationAddressKey]generationAddressReservation)
)

// StageHostRegistrationGenerationAddress records the exact address selected by
// runtime IPAM for one bridge attempt. The returned rollback is token-scoped so
// cleanup from an older attempt cannot erase a newer reservation for the same
// container ID.
func StageHostRegistrationGenerationAddress(networkName, containerID, ipAddr string) (func(), error) {
	if err := validateNetworkName(networkName); err != nil {
		return nil, err
	}
	if strings.TrimSpace(containerID) == "" {
		return nil, fmt.Errorf("container ID cannot be empty")
	}
	canonicalIP, err := canonicalIPAddress(ipAddr)
	if err != nil {
		return nil, err
	}
	token, err := newAttemptToken()
	if err != nil {
		return nil, err
	}
	key := generationAddressKey{networkName: networkName, containerID: containerID}
	generationAddressMu.Lock()
	generationAddresses[key] = generationAddressReservation{token: token, ip: canonicalIP}
	generationAddressMu.Unlock()

	return func() {
		generationAddressMu.Lock()
		defer generationAddressMu.Unlock()
		if generationAddresses[key].token == token {
			delete(generationAddresses, key)
		}
	}, nil
}

// BindHostRegistrationGeneration durably upgrades a registrar-owned DNS entry
// to exact child-generation ownership after bridge admission succeeds. Runtime
// admission reservations become visible to peers in the same atomic registry
// update that binds the exact child generation. If runtime IPAM staged an exact
// address for this attempt, that address is published by the same atomic update.
func BindHostRegistrationGeneration(networkName, containerID string, pid int, pidStartTime uint64) error {
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

	key := generationAddressKey{networkName: networkName, containerID: containerID}
	generationAddressMu.Lock()
	staged, hasStagedAddress := generationAddresses[key]
	generationAddressMu.Unlock()

	dnsMu.Lock()
	defer dnsMu.Unlock()
	dir, err := ensureDNSDir()
	if err != nil {
		return err
	}
	err = withDNSNetworkLock(dir, networkName, func(dirFD int) error {
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
				if !hasStagedAddress || entry.IP == staged.ip {
					return nil
				}
				return fmt.Errorf(
					"DNS registration for container %q generation %d/%d is already published at %s, not %s",
					containerID,
					pid,
					pidStartTime,
					entry.IP,
					staged.ip,
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
			if hasStagedAddress {
				updated[i].IP = staged.ip
			}
			updated[i].GenerationPID = pid
			updated[i].GenerationStartTime = pidStartTime
			updated[i].AdmissionPending = false
			return saveEntriesAtomicAt(dirFD, netName, networkName, updated)
		}
		return fmt.Errorf("DNS registration for container %q does not exist", containerID)
	})
	if err != nil {
		return err
	}
	if hasStagedAddress {
		generationAddressMu.Lock()
		if generationAddresses[key].token == staged.token {
			delete(generationAddresses, key)
		}
		generationAddressMu.Unlock()
	}
	return nil
}
