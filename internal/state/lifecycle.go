package state

import (
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

	c, err := s.getUnlocked(id)
	if err != nil {
		return err
	}
	if c.Status == StatusRunning && (c.PID != pid || c.PIDStartTime != pidStartTime) {
		return fmt.Errorf("container %s is already bound to running process %d/%d", id, c.PID, c.PIDStartTime)
	}

	c.PID = pid
	c.PIDStartTime = pidStartTime
	c.Status = StatusRunning
	c.StartedAt = &startedAt
	c.FinishedAt = nil
	c.ExitCode = 0

	data, err := marshalContainer(c)
	if err != nil {
		return err
	}
	return atomicWriteFile(s.ctrDir, containerStatePath(s.ctrDir, id), data)
}

// MarkStoppedIfIdentity atomically marks a running container stopped only when
// the persisted process identity still matches the caller's observation.
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

	c, err := s.getUnlocked(id)
	if err != nil {
		return false, err
	}
	if c.Status != StatusRunning || c.PID != pid || c.PIDStartTime != pidStartTime {
		return false, nil
	}

	c.Status = StatusStopped
	c.PID = 0
	c.PIDStartTime = 0
	c.FinishedAt = &finishedAt
	c.ExitCode = exitCode

	data, err := marshalContainer(c)
	if err != nil {
		return false, err
	}
	if err := atomicWriteFile(s.ctrDir, containerStatePath(s.ctrDir, id), data); err != nil {
		return false, err
	}
	return true, nil
}

func marshalContainer(c *Container) ([]byte, error) {
	data, err := jsonMarshalIndent(c)
	if err != nil {
		return nil, fmt.Errorf("marshal container: %w", err)
	}
	return data, nil
}

func containerStatePath(dir, id string) string {
	return filepathJoin(dir, id+".json")
}
