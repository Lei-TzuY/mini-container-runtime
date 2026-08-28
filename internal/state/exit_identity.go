package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const exitedIdentitySuffix = ".exit"

type exitedIdentity struct {
	PID          int    `json:"pid"`
	PIDStartTime uint64 `json:"pid_start_time"`
}

func exitedIdentityPath(containerDir, id string) string {
	return filepath.Join(containerDir, id+exitedIdentitySuffix)
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
	return atomicWriteFile(s.ctrDir, exitedIdentityPath(s.ctrDir, id), data)
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
	path := strings.TrimSuffix(containerStatePath, ".json") + exitedIdentitySuffix
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove exited process identity: %w", err)
	}
	return nil
}
