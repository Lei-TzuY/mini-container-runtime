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
	if err := atomicWriteFile(s.ctrDir, exitedIdentityPath(s.ctrDir, id), data); err != nil {
		return err
	}
	// A successful modern stop must never be mistaken for a pre-sidecar legacy
	// record if the exact identity is later lost or corrupted. Persist a durable
	// capability marker before stopped state commits; unlike the generation key,
	// this marker survives restarts and is removed only with the container.
	if err := atomicWriteFile(s.ctrDir, exitedIdentityRequiredPath(s.ctrDir, id), []byte("1\n")); err != nil {
		return fmt.Errorf("persist exited identity requirement: %w", err)
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
// current=true with ok=false denotes a stopped record without a sidecar. Such a
// record is legacy only when StoppedRevisionRequiresExitedIdentity reports
// required=false.
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
