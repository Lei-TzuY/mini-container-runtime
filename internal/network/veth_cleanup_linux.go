//go:build linux

package network

import (
	"fmt"
	"net"
	"syscall"
)

type vethInterfaceLister func() ([]net.Interface, error)
type vethLinkDeleter func(name string, index int) error

// RemoveVethHost deletes the host side of a container veth pair. Deleting one
// end of a veth pair removes its peer as well, including when the peer has
// already been moved into the container network namespace.
//
// Missing interfaces are treated as already-cleaned. Interface enumeration
// failures are not equivalent to absence and are reported so teardown cannot
// silently succeed without checking whether the owned link still exists.
func RemoveVethHost(containerPID int, debug bool) error {
	return removeVethHostWith(containerPID, debug, net.Interfaces, deleteVethLink)
}

func removeVethHostWith(containerPID int, debug bool, list vethInterfaceLister, deleteLink vethLinkDeleter) error {
	if list == nil || deleteLink == nil {
		return fmt.Errorf("veth cleanup operation is nil")
	}

	name := VethHostIface(containerPID)
	ifaces, err := list()
	if err != nil {
		return fmt.Errorf("list interfaces for veth cleanup %s: %w", name, err)
	}

	var iface *net.Interface
	for i := range ifaces {
		if ifaces[i].Name == name {
			iface = &ifaces[i]
			break
		}
	}
	if iface == nil {
		if debug {
			fmt.Printf("[parent] veth cleanup: %s already absent\n", name)
		}
		return nil
	}

	if err := deleteLink(name, iface.Index); err != nil {
		return fmt.Errorf("delete host veth %s: %w", name, err)
	}
	if debug {
		fmt.Printf("[parent] veth cleanup: removed %s\n", name)
	}
	return nil
}

func deleteVethLink(name string, index int) error {
	body := mkIfInfomsg(syscall.AF_UNSPEC, int32(index), 0, 0)
	msg := nlMsg(syscall.RTM_DELLINK, syscall.NLM_F_REQUEST|syscall.NLM_F_ACK, body)
	s, err := openNL()
	if err != nil {
		return fmt.Errorf("open netlink: %w", err)
	}
	defer s.close()
	if err := s.do(msg); err != nil {
		return err
	}
	return nil
}
