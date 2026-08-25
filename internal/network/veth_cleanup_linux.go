//go:build linux

package network

import (
	"fmt"
	"net"
	"syscall"
)

// RemoveVethHost deletes the host side of a container veth pair. Deleting one
// end of a veth pair removes its peer as well, including when the peer has
// already been moved into the container network namespace.
//
// Missing interfaces are treated as already-cleaned: callers use this helper
// both after successful setup and to roll back partially completed setup.
func RemoveVethHost(containerPID int, debug bool) error {
	name := VethHostIface(containerPID)
	iface, err := net.InterfaceByName(name)
	if err != nil {
		if debug {
			fmt.Printf("[parent] veth cleanup: %s already absent\n", name)
		}
		return nil
	}

	body := mkIfInfomsg(syscall.AF_UNSPEC, int32(iface.Index), 0, 0)
	msg := nlMsg(syscall.RTM_DELLINK, syscall.NLM_F_REQUEST|syscall.NLM_F_ACK, body)
	s, err := openNL()
	if err != nil {
		return fmt.Errorf("open netlink for veth cleanup %s: %w", name, err)
	}
	defer s.close()
	if err := s.do(msg); err != nil {
		return fmt.Errorf("delete host veth %s: %w", name, err)
	}
	if debug {
		fmt.Printf("[parent] veth cleanup: removed %s\n", name)
	}
	return nil
}
