//go:build linux

package network

import (
	"errors"
	"fmt"
)

type vethHostSetupOps struct {
	createPair func(name, peer string) error
	addAddr    func(name, cidr string) error
	setLinkUp  func(name string) error
	movePeer   func(name string, pid int) error
	removeHost func(pid int, debug bool) error
}

func defaultVethHostSetupOps() vethHostSetupOps {
	return vethHostSetupOps{
		createPair: createVethPair,
		addAddr:    addAddr,
		setLinkUp:  setLinkUp,
		movePeer:   moveToNetns,
		removeHost: RemoveVethHost,
	}
}

// SetupVethHostOwned configures the host side of a veth pair transactionally.
// A failed create does not establish ownership and therefore never triggers a
// delete. Once create succeeds, every later setup failure rolls back the exact
// veth this call created and preserves any rollback failure alongside the setup
// error.
func SetupVethHostOwned(containerPID int, hostCIDR string, debug bool) error {
	return setupVethHostOwnedWithOps(containerPID, hostCIDR, debug, defaultVethHostSetupOps())
}

func setupVethHostOwnedWithOps(containerPID int, hostCIDR string, debug bool, ops vethHostSetupOps) error {
	if ops.createPair == nil || ops.addAddr == nil || ops.setLinkUp == nil || ops.movePeer == nil || ops.removeHost == nil {
		return fmt.Errorf("veth host setup operation is nil")
	}

	host := VethHostIface(containerPID)
	if debug {
		fmt.Printf("[parent] veth: creating pair %s ↔ %s\n", host, vethPeerName)
	}
	if err := ops.createPair(host, vethPeerName); err != nil {
		return fmt.Errorf("create veth pair: %w", err)
	}

	rollback := func(setupErr error) error {
		if cleanupErr := ops.removeHost(containerPID, debug); cleanupErr != nil {
			return errors.Join(setupErr, fmt.Errorf("rollback owned host veth: %w", cleanupErr))
		}
		return setupErr
	}

	if err := ops.addAddr(host, hostCIDR); err != nil {
		return rollback(fmt.Errorf("host addr: %w", err))
	}
	if err := ops.setLinkUp(host); err != nil {
		return rollback(fmt.Errorf("host link up: %w", err))
	}
	if err := ops.movePeer(vethPeerName, containerPID); err != nil {
		return rollback(fmt.Errorf("move peer to container netns: %w", err))
	}

	if debug {
		fmt.Printf("[parent] veth: host side %s ready (%s)\n", host, hostCIDR)
	}
	return nil
}
