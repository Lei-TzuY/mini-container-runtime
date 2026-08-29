package dns

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type hostEntryActiveProbe func(HostEntry) (bool, error)

type childGenerationIdentity struct {
	PID       int
	StartTime uint64
}

func (g childGenerationIdentity) valid() bool {
	return g.PID > 0 && g.StartTime != 0
}

// CleanupStoppedHostRegistration removes service-discovery state after an
// authoritative stopped-container finalization. This compatibility entry point
// retains the registrar-scoped policy used by older callers. Runtime generation
// finalization should use CleanupStoppedHostRegistrationGeneration instead.
func CleanupStoppedHostRegistration(networkName, containerID string) error {
	return cleanupStoppedHostRegistrationForFinalization(networkName, containerID, true)
}

// CleanupStoppedHostRegistrationForFinalization lets compatibility callers
// distinguish the actor that actually committed stopped state from a later
// retry. Modern runtime finalizers should prefer exact child-generation cleanup.
func CleanupStoppedHostRegistrationForFinalization(networkName, containerID string, consumeCurrentOwner bool) error {
	return cleanupStoppedHostRegistrationForFinalization(networkName, containerID, consumeCurrentOwner)
}

// CleanupStoppedHostRegistrationGeneration removes only DNS state that can be
// proven to belong to the exact stopped child PID/start-time generation. Modern
// generation-aware entries are treated as a CAS token: an entry bound to a
// different child generation, or still unbound during a newer admission, is
// never consumed merely because its registrar matches or is stale.
//
// Exact generation teardown deliberately has no dependency on the registrar
// process identity. Crash/reconciliation cleanup may run after that registrar is
// gone or when its identity cannot be resolved; the durable child PID/start-time
// pair is the destructive authority. Generation-unaware registrar-owned entries
// are migration-only records because modern registration always writes
// generation-aware ownership. Truly ownerless legacy entries are handled by the
// separately revision-guarded ownerless migration cleanup.
func CleanupStoppedHostRegistrationGeneration(networkName, containerID string, generationPID int, generationStartTime uint64) error {
	generation := childGenerationIdentity{PID: generationPID, StartTime: generationStartTime}
	return cleanupStoppedHostRegistrationWithGenerationPolicy(networkName, containerID, generation)
}

func cleanupStoppedHostRegistrationForFinalization(networkName, containerID string, consumeCurrentOwner bool) error {
	owner, err := currentRegistrarIdentity()
	if err != nil {
		return err
	}
	return cleanupStoppedHostRegistrationWithPolicy(networkName, containerID, owner, hostEntryOwnerActive, consumeCurrentOwner)
}

func cleanupStoppedHostRegistrationWith(
	networkName, containerID string,
	currentOwner registrarIdentity,
	ownerActive hostEntryActiveProbe,
) error {
	return cleanupStoppedHostRegistrationWithPolicy(networkName, containerID, currentOwner, ownerActive, true)
}

func validateStoppedCleanupInputs(networkName, containerID string, currentOwner registrarIdentity, ownerActive hostEntryActiveProbe) error {
	if err := validateStoppedGenerationCleanupInputs(networkName, containerID); err != nil {
		return err
	}
	if currentOwner.PID <= 0 || currentOwner.StartTime == 0 {
		return fmt.Errorf("invalid current DNS registrar process identity %d/%d", currentOwner.PID, currentOwner.StartTime)
	}
	if ownerActive == nil {
		return fmt.Errorf("DNS ownership activity probe is nil")
	}
	return nil
}

func validateStoppedGenerationCleanupInputs(networkName, containerID string) error {
	if err := validateNetworkName(networkName); err != nil {
		return err
	}
	if strings.TrimSpace(containerID) == "" {
		return fmt.Errorf("container ID cannot be empty")
	}
	return nil
}

func withStoppedDNSRegistry(networkName string, mutate func([]HostEntry) ([]HostEntry, bool, error)) error {
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
		updated, changed, err := mutate(entries)
		if err != nil || !changed {
			return err
		}
		return saveEntriesAtomic(dir, netFile, networkName, updated)
	})
}

func cleanupStoppedHostRegistrationWithPolicy(
	networkName, containerID string,
	currentOwner registrarIdentity,
	ownerActive hostEntryActiveProbe,
	consumeCurrentOwner bool,
) error {
	if err := validateStoppedCleanupInputs(networkName, containerID, currentOwner, ownerActive); err != nil {
		return err
	}

	return withStoppedDNSRegistry(networkName, func(entries []HostEntry) ([]HostEntry, bool, error) {
		updated := make([]HostEntry, 0, len(entries))
		removed := false
		for _, entry := range entries {
			if entry.ContainerID != containerID {
				updated = append(updated, entry)
				continue
			}

			// This compatibility path has no child-generation proof. A modern
			// registration may have appeared after a legacy stopped snapshot was
			// validated, so registrar identity/liveness is never sufficient
			// authority to consume a generation-aware entry. Exact modern teardown
			// belongs exclusively to cleanupStoppedHostRegistrationWithGenerationPolicy.
			if entry.GenerationAware {
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
				if consumeCurrentOwner {
					removed = true
					continue
				}
				updated = append(updated, entry)
				continue
			}

			active, err := ownerActive(entry)
			if err != nil {
				return nil, false, fmt.Errorf("resolve DNS ownership before stopped cleanup for container %s: %w", containerID, err)
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

func cleanupStoppedHostRegistrationWithGenerationPolicy(
	networkName, containerID string,
	generation childGenerationIdentity,
) error {
	if err := validateStoppedGenerationCleanupInputs(networkName, containerID); err != nil {
		return err
	}
	if !generation.valid() {
		return fmt.Errorf("invalid DNS child process identity %d/%d", generation.PID, generation.StartTime)
	}

	return withStoppedDNSRegistry(networkName, func(entries []HostEntry) ([]HostEntry, bool, error) {
		updated := make([]HostEntry, 0, len(entries))
		removed := false
		for _, entry := range entries {
			if entry.ContainerID != containerID {
				updated = append(updated, entry)
				continue
			}

			if entry.GenerationAware {
				// Unbound means admission has not durably attached this registration
				// to any child generation yet. It may already be the next restart
				// attempt, so a stale finalizer must preserve it.
				if entry.GenerationPID == 0 && entry.GenerationStartTime == 0 {
					updated = append(updated, entry)
					continue
				}
				if entry.GenerationPID == generation.PID && entry.GenerationStartTime == generation.StartTime {
					removed = true
					continue
				}
				// Exact modern generation mismatch is authoritative evidence that
				// this entry belongs to another runtime generation. Never fall back
				// to registrar liveness and accidentally consume the replacement.
				updated = append(updated, entry)
				continue
			}

			// Truly legacy entries have no ownership proof at all. They are
			// retired by the separately revision-guarded ownerless migration path.
			if entry.OwnerPID == 0 && entry.OwnerStartTime == 0 {
				updated = append(updated, entry)
				continue
			}

			// Generation-unaware registrar ownership can only come from a legacy
			// runtime: modern admission always writes GenerationAware=true. Once the
			// caller has established exact stopped-generation authority, retaining
			// this migration record based on registrar liveness can only prolong a
			// stale hostname reservation. A concurrent modern restart is still safe:
			// its generation-aware replacement is preserved by the branch above.
			removed = true
		}
		return updated, removed, nil
	})
}
