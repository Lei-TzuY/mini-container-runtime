//go:build linux

package network

import "fmt"

const vethPeerHandoffLockName = "_veth-peer-handoff"

// withVethPeerHandoffLock serializes the interval where the fixed veth peer
// name exists in the host network namespace. Independent minictl processes
// share the same kernel flock through the runtime IPAM directory, so one
// generation must move its peer into the target netns before another creates
// the next fixed-name peer.
func withVethPeerHandoffLock(fn func() error) error {
	if fn == nil {
		return fmt.Errorf("veth peer handoff callback is nil")
	}
	ipam, err := OpenIPAM(DefaultIPAMDir())
	if err != nil {
		return fmt.Errorf("open veth peer handoff lock directory: %w", err)
	}
	return withIPAMNetworkLock(ipam.dir, vethPeerHandoffLockName, fn)
}
