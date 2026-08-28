package dns

import (
	"errors"
	"fmt"
	"os"

	"minicontainer/internal/state"
)

// hostEntryOwnerActive decides whether a process-owned DNS entry is still
// authoritative. Modern registrations are owned by the exact child generation
// once that generation is durably bound after bridge admission; registrar
// liveness matters only while a modern registration is still unbound. Legacy
// registrations retain the older registrar/state/network proof path for
// backward compatibility. State/probe uncertainty fails closed rather than
// deleting a possibly-live container's discovery record.
func hostEntryOwnerActive(entry HostEntry) (bool, error) {
	if entry.GenerationAware {
		boundPID := entry.GenerationPID != 0
		boundStart := entry.GenerationStartTime != 0
		if boundPID != boundStart {
			return false, fmt.Errorf(
				"DNS entry for container %s has incomplete child generation identity %d/%d",
				entry.ContainerID,
				entry.GenerationPID,
				entry.GenerationStartTime,
			)
		}
		if boundPID {
			alive, err := registrarGenerationAlive(entry.GenerationPID, entry.GenerationStartTime)
			if err != nil {
				return false, fmt.Errorf(
					"probe DNS child generation %s %d/%d: %w",
					entry.ContainerID,
					entry.GenerationPID,
					entry.GenerationStartTime,
					err,
				)
			}
			return alive, nil
		}

		// A modern entry is published before the child can be durably bound. In
		// that narrow admission window the registrar is the only authority that
		// can still complete or roll back setup. Once the registrar dies, an
		// unbound entry must not be adopted from indirect lifecycle state.
		alive, err := registrarGenerationAlive(entry.OwnerPID, entry.OwnerStartTime)
		if err != nil {
			return false, err
		}
		return alive, nil
	}

	alive, err := registrarGenerationAlive(entry.OwnerPID, entry.OwnerStartTime)
	if err != nil {
		return false, err
	}
	if alive {
		return true, nil
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
		// Container IDs are reusable across stopped/delete/recreate cycles. A
		// newer generation with the same ID must not accidentally adopt an old
		// registrar's service-discovery name.
		if c.Hostname != entry.Hostname {
			return false, nil
		}

		// MarkRunning commits before bridge setup. For legacy entries, matching
		// durable bridge ownership remains the strongest available adoption proof.
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
