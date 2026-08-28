package dns

import (
	"errors"
	"fmt"
	"os"

	"minicontainer/internal/state"
)

// hostEntryOwnerActive decides whether a process-owned DNS entry is still
// authoritative. The registrar process owns setup and the whole restart loop.
// If that process is gone, only a committed live container generation with
// matching durable bridge-network ownership may adopt the registration.
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

		// MarkRunning commits before bridge setup. If the parent crashes in that
		// window, the blocked child has not acquired host-network resources and
		// must not make the pre-generation DNS registration authoritative. The
		// generation-scoped network sidecar is persisted before bridge mutation,
		// so it is the durable proof that this running generation reached bridge
		// admission.
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
		// A generation-scoped host veth is direct bridge-admission proof. Older
		// runtimes may leave rules-only sidecars, so preserve compatibility when
		// at least one owned DNAT rule explicitly targets the same container IP
		// as this DNS entry. A same-generation sidecar describing unrelated host
		// networking must not be allowed to adopt the service-discovery record.
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
