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

func validateExitedIdentity(identity exitedIdentity) error {
	if identity.PID <= 0 || identity.PIDStartTime == 0 {
		return fmt.Errorf("invalid persisted exited process identity %d/%d", identity.PID, identity.PIDStartTime)
	}
	return nil
}

func exitedIdentityPath(containerDir, id string) string {
	return filepath.Join(containerDir, id+exitedIdentitySuffix)
}

func exitedIdentityRequiredPath(containerDir, id string) string {
	return filepath.Join(containerDir, id+exitedIdentityRequiredSuffix)
}

// writeExitedIdentityUnlocked is retained for upgrade-compatibility tests and
// legacy records. Modern stopped transitions embed the exact identity directly
// in the atomic container JSON and never publish a new .exit sidecar.
func (s *Store) writeExitedIdentityUnlocked(id string, pid int, pidStartTime uint64) error {
	if err := validateID(id); err != nil {
		return err
	}
	identity := exitedIdentity{PID: pid, PIDStartTime: pidStartTime}
	if err := validateExitedIdentity(identity); err != nil {
		return err
	}
	data, err := json.Marshal(identity)
	if err != nil {
		return fmt.Errorf("marshal exited process identity: %w", err)
	}
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
	if err := validateExitedIdentity(identity); err != nil {
		return exitedIdentity{}, false, err
	}
	return identity, true, nil
}

// containerEmbeddedExitedIdentityUnlocked reads only the modern in-JSON exact
// generation key. A present-but-null, malformed, or invalid field is corruption
// and must fail closed rather than falling back to a legacy sidecar.
func (s *Store) containerEmbeddedExitedIdentityUnlocked(id string) (exitedIdentity, bool, error) {
	if err := validateID(id); err != nil {
		return exitedIdentity{}, false, err
	}
	path := filepath.Join(s.ctrDir, id+".json")
	data, err := readRegularStateFile(path, "container state")
	if err != nil {
		return exitedIdentity{}, false, err
	}
	var metadata struct {
		ExitIdentity json.RawMessage `json:"exit_identity"`
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return exitedIdentity{}, false, fmt.Errorf("unmarshal container exit identity: %w", err)
	}
	if len(metadata.ExitIdentity) == 0 {
		return exitedIdentity{}, false, nil
	}
	if string(metadata.ExitIdentity) == "null" {
		return exitedIdentity{}, false, fmt.Errorf("invalid persisted exit identity: null")
	}
	var identity exitedIdentity
	if err := json.Unmarshal(metadata.ExitIdentity, &identity); err != nil {
		return exitedIdentity{}, false, fmt.Errorf("unmarshal embedded exited process identity: %w", err)
	}
	if err := validateExitedIdentity(identity); err != nil {
		return exitedIdentity{}, false, err
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

// readCurrentExitedIdentityUnlocked prefers the lifecycle JSON. Legacy .exit is
// consulted only when the embedded field is genuinely absent. Corrupt embedded
// metadata never falls back to a potentially stale sidecar.
func (s *Store) readCurrentExitedIdentityUnlocked(id string) (exitedIdentity, bool, error) {
	identity, embedded, err := s.containerEmbeddedExitedIdentityUnlocked(id)
	if err != nil {
		return exitedIdentity{}, false, err
	}
	if embedded {
		required, present, err := s.containerExitIdentityRequirementUnlocked(id)
		if err != nil {
			return exitedIdentity{}, false, err
		}
		if !present || !required {
			return exitedIdentity{}, false, fmt.Errorf("persisted exit identity exists without required lifecycle capability")
		}
		return identity, true, nil
	}
	return s.readExitedIdentityUnlocked(id)
}

// GetExitedIdentity returns the durable PID/start-time identity of the process
// that produced the current stopped generation. Modern records read it from the
// container JSON; legacy .exit sidecars remain read-only upgrade compatibility.
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

	identity, ok, err := s.readCurrentExitedIdentityUnlocked(id)
	if err != nil || !ok {
		return 0, 0, ok, err
	}
	return identity.PID, identity.PIDStartTime, true, nil
}

// GetExitedIdentityForStoppedRevision atomically validates that revision still
// names the current stopped lifecycle and only then reads its durable exact
// generation identity. current=false is a stale-snapshot no-op.
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

	identity, ok, err := s.readCurrentExitedIdentityUnlocked(id)
	if err != nil || !ok {
		return 0, 0, true, ok, err
	}
	return identity.PID, identity.PIDStartTime, true, true, nil
}

// GetStoppedExitIdentityPolicy validates one stopped revision and reads its
// exact identity plus legacy-compatibility policy under the same state lock.
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

	required, present, err := s.containerExitIdentityRequirementUnlocked(id)
	if err != nil {
		return 0, 0, true, false, false, err
	}
	identity, embedded, err := s.containerEmbeddedExitedIdentityUnlocked(id)
	if err != nil {
		return 0, 0, true, false, false, err
	}
	if embedded {
		if !present || !required {
			return 0, 0, true, false, false, fmt.Errorf("persisted exit identity exists without required lifecycle capability")
		}
		return identity.PID, identity.PIDStartTime, true, true, true, nil
	}

	// Upgrade compatibility for #247-era and older stopped records: only a
	// genuinely absent embedded field may consult the historical .exit sidecar.
	identity, ok, err = s.readExitedIdentityUnlocked(id)
	if err != nil {
		return 0, 0, true, false, false, err
	}
	if ok {
		return identity.PID, identity.PIDStartTime, true, true, true, nil
	}
	if present {
		return 0, 0, true, false, required, nil
	}

	// Older releases used an .exit-required marker. Its presence must continue
	// to make a missing identity fail closed rather than grant legacy cleanup.
	required, err = s.exitedIdentityRequiredUnlocked(id)
	if err != nil {
		return 0, 0, true, false, false, err
	}
	return 0, 0, true, false, required, nil
}

// StoppedRevisionRequiresExitedIdentity revalidates a stopped snapshot and
// reports whether it belongs to a runtime generation that requires exact exit
// identity. current=false means the caller's snapshot became stale.
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
