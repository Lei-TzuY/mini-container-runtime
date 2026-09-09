package network

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

// ParseBridgeLeaseOwner decodes the durable owner key used for generation-scoped
// bridge leases. Legacy/non-generation IPAM owners are deliberately rejected so
// reconciliation cannot reclaim allocations it does not understand.
func ParseBridgeLeaseOwner(key string) (BridgeLeaseOwner, error) {
	const prefix = "generation:"
	if !strings.HasPrefix(key, prefix) {
		return BridgeLeaseOwner{}, fmt.Errorf("not a generation bridge lease owner")
	}
	parts := strings.Split(strings.TrimPrefix(key, prefix), ":")
	if len(parts) != 3 {
		return BridgeLeaseOwner{}, fmt.Errorf("invalid generation bridge lease owner %q", key)
	}
	id, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return BridgeLeaseOwner{}, fmt.Errorf("decode bridge lease container ID: %w", err)
	}
	pid, err := strconv.Atoi(parts[1])
	if err != nil {
		return BridgeLeaseOwner{}, fmt.Errorf("parse bridge lease PID: %w", err)
	}
	start, err := strconv.ParseUint(parts[2], 10, 64)
	if err != nil {
		return BridgeLeaseOwner{}, fmt.Errorf("parse bridge lease PID start time: %w", err)
	}
	owner := BridgeLeaseOwner{ContainerID: string(id), PID: pid, PIDStartTime: start}
	if _, err := owner.key(); err != nil {
		return BridgeLeaseOwner{}, err
	}
	return owner, nil
}

// ReconcileGenerationIPs reclaims generation-scoped leases whose exact process
// generation is no longer alive. The liveness callback must compare both PID
// and PID start time; this keeps PID reuse from freeing a newer generation.
// Unknown/legacy owners are preserved.
func (ipam *IPAM) ReconcileGenerationIPs(networkName string, alive func(BridgeLeaseOwner) bool) (int, error) {
	if err := ipam.ready(); err != nil {
		return 0, err
	}
	if err := validateNetworkName(networkName); err != nil {
		return 0, err
	}
	if alive == nil {
		return 0, fmt.Errorf("bridge lease liveness callback is nil")
	}

	ipam.mu.Lock()
	defer ipam.mu.Unlock()

	reclaimed := 0
	err := withIPAMNetworkLock(ipam.dir, networkName, func() error {
		pool, exists, err := ipam.loadExistingPool(networkName)
		if err != nil || !exists {
			return err
		}
		for ip, key := range pool.Allocated {
			owner, err := ParseBridgeLeaseOwner(key)
			if err != nil {
				continue
			}
			if !alive(owner) {
				delete(pool.Allocated, ip)
				reclaimed++
			}
		}
		if reclaimed == 0 {
			return nil
		}
		if err := ipam.savePool(networkName, pool); err != nil {
			return fmt.Errorf("save reconciled IPAM pool for %q: %w", networkName, err)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return reclaimed, nil
}
