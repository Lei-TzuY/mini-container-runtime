package state

import (
	"errors"
	"fmt"
	"time"
)

// MarkRunning atomically transitions an existing container record to running
// and binds it to a specific host process identity (PID + Linux starttime).
func (s *Store) MarkRunning(id string, pid int, pidStartTime uint64, startedAt time.Time) error {
	if s == nil {
		return fmt.Errorf("state store is nil")
	}
	if err := validateID(id); err != nil {
		return err
	}
	if pid <= 0 {
		return fmt.Errorf("invalid container PID %d", pid)
	}
	if pidStartTime == 0 {
		return fmt.Errorf("process start time must be non-zero")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := lockStateFile(s.lockFile); err != nil {
		return err
	}
	defer func() { _ = unlockStateFile(s.lockFile) }()

	c, err := s.getUnlocked(id)
	if err != nil {
		return err
	}
	if c.Status == StatusRunning && (c.PID != pid || c.PIDStartTime != pidStartTime) {
		return fmt.Errorf("container %s is already bound to running process %d/%d", id, c.PID, c.PIDStartTime)
	}

	previousExitIdentity, hadPreviousExitIdentity, err := s.readExitedIdentityUnlocked(id)
	if err != nil {
		return fmt.Errorf("read prior exited identity before start: %w", err)
	}
	if err := s.clearExitedIdentityUnlocked(id); err != nil {
		return fmt.Errorf("clear prior exited identity before start: %w", err)
	}

	c.PID = pid
	c.PIDStartTime = pidStartTime
	c.Status = StatusRunning
	c.StartedAt = &startedAt
	c.FinishedAt = nil
	c.ExitCode = 0

	if err := s.writeContainerNextRevisionUnlocked(c); err != nil {
		if hadPreviousExitIdentity {
			restoreErr := s.writeExitedIdentityUnlocked(id, previousExitIdentity.PID, previousExitIdentity.PIDStartTime)
			if restoreErr != nil {
				return errors.Join(err, fmt.Errorf("restore prior exited identity after failed start transition: %w", restoreErr))
			}
		}
		return err
	}
	return nil
}

// MarkStoppedIfIdentity atomically marks a running container stopped only when
// the persisted process identity still matches the caller's observation.
//
// An observer that can prove only that the process exited may persist exitCode
// -1. In that case a private exited-identity tombstone is retained so the
// process-owning reaper can later upgrade the unknown code for the exact same
// PID/start-time lifecycle. A different or restarted process identity can never
// use the tombstone to overwrite the current lifecycle.
//
// Returning changed=false is intentional: another lifecycle actor may already
// have stopped/restarted the container, and stale observations must not win.
func (s *Store) MarkStoppedIfIdentity(id string, pid int, pidStartTime uint64, exitCode int, finishedAt time.Time) (changed bool, err error) {
	if s == nil {
		return false, fmt.Errorf("state store is nil")
	}
	if err := validateID(id); err != nil {
		return false, err
	}
	if pid <= 0 || pidStartTime == 0 {
		return false, fmt.Errorf("invalid process identity %d/%d", pid, pidStartTime)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := lockStateFile(s.lockFile); err != nil {
		return false, err
	}
	defer func() { _ = unlockStateFile(s.lockFile) }()

	c, err := s.getUnlocked(id)
	if err != nil {
		return false, err
	}

	// A non-owning observer may have won the stopped-state race with an unknown
	// exit code. Permit one later authoritative upgrade only when the durable
	// tombstone proves it refers to the same exited process identity.
	if c.Status == StatusStopped {
		if c.ExitCode != -1 || exitCode == -1 {
			return false, nil
		}
		exited, ok, err := s.readExitedIdentityUnlocked(id)
		if err != nil {
			return false, fmt.Errorf("read exited identity for exit-code reconciliation: %w", err)
		}
		if !ok || exited.PID != pid || exited.PIDStartTime != pidStartTime {
			return false, nil
		}

		c.FinishedAt = &finishedAt
		c.ExitCode = exitCode
		if err := s.writeContainerNextRevisionUnlocked(c); err != nil {
			return false, err
		}
		if err := s.clearExitedIdentityUnlocked(id); err != nil {
			return true, fmt.Errorf("clear reconciled exited identity: %w", err)
		}
		return true, nil
	}

	if c.Status != StatusRunning || c.PID != pid || c.PIDStartTime != pidStartTime {
		return false, nil
	}

	wroteUnknownIdentity := false
	if exitCode == -1 {
		if err := s.writeExitedIdentityUnlocked(id, pid, pidStartTime); err != nil {
			return false, fmt.Errorf("persist exited identity before unknown exit status: %w", err)
		}
		wroteUnknownIdentity = true
	}

	c.Status = StatusStopped
	c.PID = 0
	c.PIDStartTime = 0
	c.FinishedAt = &finishedAt
	c.ExitCode = exitCode

	if err := s.writeContainerNextRevisionUnlocked(c); err != nil {
		if wroteUnknownIdentity {
			clearErr := s.clearExitedIdentityUnlocked(id)
			if clearErr != nil {
				return false, errors.Join(err, fmt.Errorf("rollback exited identity after failed stop transition: %w", clearErr))
			}
		}
		return false, err
	}
	if !wroteUnknownIdentity {
		if err := s.clearExitedIdentityUnlocked(id); err != nil {
			return true, fmt.Errorf("clear stale exited identity after authoritative stop: %w", err)
		}
	}
	return true, nil
}
