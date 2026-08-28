package dns

import (
	"errors"
	"fmt"
	"os"

	"minicontainer/internal/state"
)

// hostEntryOwnerActive decides whether a process-owned DNS entry is still
// authoritative. The registrar process owns setup and the whole restart loop.
// If that process is gone, modern registrations may be adopted only by the exact
// child generation durably bound after bridge admission. Legacy registrations
// retain the older state/network proof path for backward compatibility.
// State/probe uncertainty fails closed rather than deleting a possibly-live
// container's discovery record.
func hostEntryOwnerActive(entry HostEntry) (bool, error) {
	alive, err := registrarGenerationAlive(entry.OwnerPID, entry.OwnerStartTime)
	if err != nil {
		return false, err
	}
	if alive {
		return true, nil
	}

	// New registrations explicitly distinguish "child not durably admitted yet"
	// from legacy registry entries that predate child-generation ownership. If a
	// modern registrar dies before binding its child, the entry has no authority
	// to survive merely because a running-state/network reservation exists.
	if entry.GenerationAware && entry.GenerationPID == 0 && entry.GenerationStartTime == 0 {
		return false, nil
	}

	st, err := state.Open(state.DefaultDir())
	if err != nil {
		return false, fmt.Errorf("open state while recovering DNS owner for container %s: %w", entry.ContainerID, err)
	}
	defer st.Close()

	c, err := st.Get(entry.ContainerID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("read state while recovering DNS owner for container %s: %w", entry.ContainerID, err)
	}
	switch c.Status {
	case state.StatusRunning:
		if c.PID <= 0 || c.PIDStartTime == 0 {
			return false, fmt.Errorf("running container %s has invalid process identity %d/%d", c.ID, c.PID, c.PIDStartTime)
		}
		if entry.GenerationAware && (c.PID != entry.GenerationPID || c.PIDStartTime != entry.GenerationStartTime) {
			return false, nil
		}
		// Container IDs are reusable across stopped/delete/recreate cycles. A
		// newer generation with the same ID must not accidentally adopt an old
		// registrar's service-discovery name.
		if c.Hostname != entry.Hostname {
			return false, nil
		}

		// MarkRunning commits before bridge setup. For legacy entries, matching
		// durable bridge ownership remains the strongest available adoption proof.
		// Modern entries additionally require the exact child-generation binding
		// checked above, which is published only after bridge setup succeeds.
		networkOwner, ok, err := st.GetNetworkOwnership(c.ID)
		if err != nil {
			return false, fmt.Errorf("read network ownership while recovering DNS owner for container %s: %w", c.ID, err)
		}
		if !ok {
			return false, nil
		}
		if networkOwner.PID != c.PID || networkOwner.PIDStartTime != c.PIDStartTime {
			return false, fmt.Errorf(
				"network ownership for container %s belongs to process %d/%d, not running generation %d/%d",
				c.ID,
				networkOwner.PID,
				networkOwner.PIDStartTime,
				c.PID,
				c.PIDStartTime,
			)
		}
		bridgeProof := networkOwner.VethHost != ""
		if !bridgeProof {
			for _, mapping := range networkOwner.Mappings {
				if mapping.ContainerIP == entry.IP {
					bridgeProof = true
					break
				}
			}
		}
		if !bridgeProof {
			return false, nil
		}

		childAlive, err := registrarGenerationAlive(c.PID, c.PIDStartTime)
		if err != nil {
			return false, fmt.Errorf("probe adopted container generation %s %d/%d: %w", c.ID, c.PID, c.PIDStartTime, err)
		}
		return childAlive, nil
	case state.StatusCreated, state.StatusStopped:
		return false, nil
	default:
		return false, fmt.Errorf("container %s has unknown lifecycle status %q while recovering DNS ownership", c.ID, c.Status)
	}
}
