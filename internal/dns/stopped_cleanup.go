package dns

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type childGenerationIdentity struct {
	PID       int
	StartTime uint64
}

func (g childGenerationIdentity) valid() bool {
	return g.PID > 0 && g.StartTime != 0
}

// CleanupStoppedHostRegistrationGeneration removes DNS state that can be
// retired under authority for one exact stopped child PID/start-time generation.
// Modern generation-aware entries are treated as a CAS token: an entry bound to
// a different child generation, or still unbound during a newer admission, is
// never consumed merely because its registrar matches or is stale.
//
// Exact generation teardown deliberately has no dependency on the registrar
// process identity. Crash/reconciliation cleanup may run after that registrar is
// gone or when its identity cannot be resolved; the durable child PID/start-time
// pair is the destructive authority. Generation-unaware entries are migration
// debris because current admission always writes generation-aware ownership;
// once the caller has exact stopped-generation authority, both registrar-owned
// and pre-ownership ownerless legacy records can be retired in this same registry
// transaction without granting authority over a concurrent modern restart.
func CleanupStoppedHostRegistrationGeneration(networkName, containerID string, generationPID int, generationStartTime uint64) error {
	generation := childGenerationIdentity{PID: generationPID, StartTime: generationStartTime}
	return cleanupStoppedHostRegistrationWithGenerationPolicy(networkName, containerID, generation)
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

			// Current admission never creates generation-unaware records. With an
			// exact stopped-generation proof, every such record for this container
			// is migration debris, whether registrar-owned or pre-ownership. Retire
			// it inside this transaction instead of opening a second ownerless-only
			// cleanup window after the modern CAS decision.
			removed = true
		}
		return updated, removed, nil
	})
}
