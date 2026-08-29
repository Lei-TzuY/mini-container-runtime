package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	exitedIdentitySuffix         = ".exit"
	exitedIdentityRequiredSuffix = ".exit-required"
)

type exitedIdentity struct {
	PID          int    `json:"pid"`
	PIDStartTime uint64 `json:"pid_start_time"`
}

func exitedIdentityPath(containerDir, id string) string {
	return filepath.Join(containerDir, id+exitedIdentitySuffix)
}

func exitedIdentityRequiredPath(containerDir, id string) string {
	return filepath.Join(containerDir, id+exitedIdentityRequiredSuffix)
}

func (s *Store) writeExitedIdentityUnlocked(id string, pid int, pidStartTime uint64) error {
	if err := validateID(id); err != nil {
		return err
	}
	if pid <= 0 || pidStartTime == 0 {
		return fmt.Errorf("invalid exited process identity %d/%d", pid, pidStartTime)
	}
	data, err := json.Marshal(exitedIdentity{PID: pid, PIDStartTime: pidStartTime})
	if err != nil {
		return fmt.Errorf("marshal exited process identity: %w", err)
	}

	// Modern stopped-state capability is committed in the container JSON. The
	// exact generation key is published first and rolled back if that JSON
	// commit fails, eliminating the old cross-file .exit-required publication.
	if err := atomicWriteFile(s.ctrDir, exitedIdentityPath(s.ctrDir, id), data); err != nil {
		return fmt.Errorf("persist exited process identity: %w", err)
	}
	return nil
}

func (s *Store) readExitedIdentityUnlocked(id string) (exitedIdentity, bool, error) {
	if err := validateID(id); err != nil {
		return exitedIdentity{}, false, err
	}
	path := exitedIdentityPath(s.ctrDir, id)
	data, err := readRegularStateFile(path, "exited process identity")
	if err != nil {
		if os.IsNotExist(err) {
			return exitedIdentity{}, false, nil
		}
		return exitedIdentity{}, false, fmt.Errorf("read exited process identity: %w", err)
	}
	var identity exitedIdentity
	if err := json.Unmarshal(data, &identity); err != nil {
		return exitedIdentity{}, false, fmt.Errorf("unmarshal exited process identity: %w", err)
	}
	if identity.PID <= 0 || identity.PIDStartTime == 0 {
		return exitedIdentity{}, false, fmt.Errorf("invalid persisted exited process identity %d/%d", identity.PID, identity.PIDStartTime)
	}
	return identity, true, nil
}

// containerExitIdentityRequirementUnlocked reads the lifecycle capability from
// the container JSON itself. present=false identifies records written before the
// in-JSON capability existed. An explicit false is rejected rather than being
// allowed to downgrade teardown authority.
func (s *Store) containerExitIdentityRequirementUnlocked(id string) (required bool, present bool, err error) {
	if err := validateID(id); err != nil {
		return false, false, err
	}
	path := filepath.Join(s.ctrDir, id+".json")
	data, err := readRegularStateFile(path, "container state")
	if err != nil {
		return false, false, err
	}
	var policy struct {
		ExitIdentityRequired *bool `json:"exit_identity_required"`
	}
	if err := json.Unmarshal(data, &policy); err != nil {
		return false, false, fmt.Errorf("unmarshal container exit identity policy: %w", err)
	}
	if policy.ExitIdentityRequired == nil {
		return false, false, nil
	}
	if !*policy.ExitIdentityRequired {
		return false, true, fmt.Errorf("invalid persisted exit identity requirement: false")
	}
	return true, true, nil
}

// exitedIdentityRequiredUnlocked is retained as read-only upgrade compatibility
// for containers stopped by releases that persisted the capability in a
// .exit-required sidecar. New lifecycle transitions never create this marker.
func (s *Store) exitedIdentityRequiredUnlocked(id string) (bool, error) {
	if err := validateID(id); err != nil {
		return false, err
	}
	path := exitedIdentityRequiredPath(s.ctrDir, id)
	data, err := readRegularStateFile(path, "exited identity requirement")
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("read exited identity requirement: %w", err)
	}
	if string(data) != "1\n" {
		return false, fmt.Errorf("invalid persisted exited identity requirement")
	}
	return true, nil
}

// GetExitedIdentity returns the durable PID/start-time identity of the process
// that produced the current stopped generation. The identity survives normal
// exit-code reconciliation and is cleared only after a later running generation
// is durable, allowing crash-retry teardown to remain generation-scoped.
func (s *Store) GetExitedIdentity(id string) (pid int, pidStartTime uint64, ok bool, err error) {
	if s == nil {
		return 0, 0, false, fmt.Errorf("state store is nil")
	}
	if err := validateID(id); err != nil {
		return 0, 0, false, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := lockStateFile(s.lockFile); err != nil {
		return 0, 0, false, err
	}
	defer func() { _ = unlockStateFile(s.lockFile) }()

	identity, ok, err := s.readExitedIdentityUnlocked(id)
	if err != nil || !ok {
		return 0, 0, ok, err
	}
	return identity.PID, identity.PIDStartTime, true, nil
}

// GetExitedIdentityForStoppedRevision atomically validates that revision still
// names the current stopped lifecycle and, only then, reads its durable exited
// PID/start-time identity. Callers use current=false as a stale-snapshot no-op;
// current=true with ok=false denotes a stopped record without a sidecar.
func (s *Store) GetExitedIdentityForStoppedRevision(id string, revision uint64) (pid int, pidStartTime uint64, current bool, ok bool, err error) {
	if s == nil {
		return 0, 0, false, false, fmt.Errorf("state store is nil")
	}
	if err := validateID(id); err != nil {
		return 0, 0, false, false, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := lockStateFile(s.lockFile); err != nil {
		return 0, 0, false, false, err
	}
	defer func() { _ = unlockStateFile(s.lockFile) }()

	c, err := s.getUnlocked(id)
	if err != nil {
		return 0, 0, false, false, err
	}
	if c.Status != StatusStopped || c.Revision != revision {
		return 0, 0, false, false, nil
	}

	identity, ok, err := s.readExitedIdentityUnlocked(id)
	if err != nil || !ok {
		return 0, 0, true, ok, err
	}
	return identity.PID, identity.PIDStartTime, true, true, nil
}

// GetStoppedExitIdentityPolicy validates one stopped revision and reads both its
// exact exited process identity and its legacy-compatibility policy under the
// same state lock. This prevents cleanup callers from having to drop the lock
// between observing a missing .exit sidecar and deciding whether that absence
// is historical or fail-closed modern state.
//
// current=false means the supplied stopped snapshot is stale. When current is
// true, ok reports whether an exact PID/start-time identity exists. required is
// meaningful when ok=false: required=true means the missing identity is
// corruption and must not acquire legacy migration cleanup authority.
func (s *Store) GetStoppedExitIdentityPolicy(id string, revision uint64) (pid int, pidStartTime uint64, current bool, ok bool, required bool, err error) {
	if s == nil {
		return 0, 0, false, false, false, fmt.Errorf("state store is nil")
	}
	if err := validateID(id); err != nil {
		return 0, 0, false, false, false, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := lockStateFile(s.lockFile); err != nil {
		return 0, 0, false, false, false, err
	}
	defer func() { _ = unlockStateFile(s.lockFile) }()

	c, err := s.getUnlocked(id)
	if err != nil {
		return 0, 0, false, false, false, err
	}
	if c.Status != StatusStopped || c.Revision != revision {
		return 0, 0, false, false, false, nil
	}

	identity, ok, err := s.readExitedIdentityUnlocked(id)
	if err != nil {
		return 0, 0, true, false, false, err
	}
	if ok {
		return identity.PID, identity.PIDStartTime, true, true, true, nil
	}

	required, present, err := s.containerExitIdentityRequirementUnlocked(id)
	if err != nil {
		return 0, 0, true, false, false, err
	}
	if present {
		return 0, 0, true, false, required, nil
	}

	// Upgrade compatibility: releases before the in-JSON capability used the
	// sidecar marker. Its presence must remain fail-closed even when .exit is
	// missing; otherwise an upgrade could accidentally grant legacy cleanup.
	required, err = s.exitedIdentityRequiredUnlocked(id)
	if err != nil {
		return 0, 0, true, false, false, err
	}
	return 0, 0, true, false, required, nil
}

// StoppedRevisionRequiresExitedIdentity revalidates a stopped snapshot and
// reports whether it belongs to a runtime generation that supports durable exit
// identities. current=false means the caller's snapshot became stale. A current
// stopped revision with required=true but no .exit sidecar is corruption and
// must fail closed rather than acquiring legacy migration authority.
func (s *Store) StoppedRevisionRequiresExitedIdentity(id string, revision uint64) (current bool, required bool, err error) {
	if s == nil {
		return false, false, fmt.Errorf("state store is nil")
	}
	if err := validateID(id); err != nil {
		return false, false, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := lockStateFile(s.lockFile); err != nil {
		return false, false, err
	}
	defer func() { _ = unlockStateFile(s.lockFile) }()

	c, err := s.getUnlocked(id)
	if err != nil {
		return false, false, err
	}
	if c.Status != StatusStopped || c.Revision != revision {
		return false, false, nil
	}
	required, present, err := s.containerExitIdentityRequirementUnlocked(id)
	if err != nil {
		return true, false, err
	}
	if present {
		return true, required, nil
	}
	required, err = s.exitedIdentityRequiredUnlocked(id)
	if err != nil {
		return true, false, err
	}
	return true, required, nil
}

func (s *Store) clearExitedIdentityUnlocked(id string) error {
	if err := validateID(id); err != nil {
		return err
	}
	return removeStateFileDurable(s.ctrDir, exitedIdentityPath(s.ctrDir, id), "exited process identity")
}

func removeExitedIdentityForContainerState(containerStatePath string) error {
	if !strings.HasSuffix(containerStatePath, ".json") {
		return nil
	}
	base := strings.TrimSuffix(containerStatePath, ".json")
	for path, label := range map[string]string{
		base + exitedIdentitySuffix:         "exited process identity",
		base + exitedIdentityRequiredSuffix: "exited identity requirement",
	} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", label, err)
		}
	}
	return nil
}
