package network

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// BridgeLeaseOwner identifies one exact container process generation.
// PIDStartTime distinguishes PID reuse, so stale cleanup cannot release a
// lease that belongs to a newer process generation of the same container.
type BridgeLeaseOwner struct {
	ContainerID   string
	PID           int
	PIDStartTime  uint64
}

func (o BridgeLeaseOwner) key() (string, error) {
	if strings.TrimSpace(o.ContainerID) == "" {
		return "", fmt.Errorf("bridge lease container ID cannot be empty")
	}
	if o.PID <= 0 || o.PIDStartTime == 0 {
		return "", fmt.Errorf("invalid bridge lease process identity %d/%d", o.PID, o.PIDStartTime)
	}
	encodedID := base64.RawURLEncoding.EncodeToString([]byte(o.ContainerID))
	return fmt.Sprintf("generation:%s:%d:%d", encodedID, o.PID, o.PIDStartTime), nil
}

// AllocateGenerationIP reserves an address for one exact process generation.
// It reuses IPAM's per-network cross-process lock and durable pool update, so
// concurrent runtime processes cannot allocate the same address.
func (ipam *IPAM) AllocateGenerationIP(networkName, cidr string, owner BridgeLeaseOwner) (string, error) {
	key, err := owner.key()
	if err != nil {
		return "", err
	}
	return ipam.AllocateIP(networkName, cidr, key)
}

// ReleaseGenerationIP releases only the exact generation's lease. A stale
// generation therefore cannot free an address owned by a newer generation of
// the same container ID.
func (ipam *IPAM) ReleaseGenerationIP(networkName string, owner BridgeLeaseOwner) error {
	key, err := owner.key()
	if err != nil {
		return err
	}
	return ipam.ReleaseIP(networkName, key)
}
