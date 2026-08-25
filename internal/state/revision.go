package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

// ErrRevisionConflict means the caller is trying to persist a stale container
// snapshot. Callers must reload the current record instead of overwriting a
// newer lifecycle transition or recreating a record that was deleted.
var ErrRevisionConflict = errors.New("container state revision conflict")

func (s *Store) saveContainerCASUnlocked(c *Container) error {
	if c == nil {
		return fmt.Errorf("container state is nil")
	}
	target := filepath.Join(s.ctrDir, c.ID+".json")
	nextRevision := uint64(1)

	data, err := os.ReadFile(target)
	switch {
	case err == nil:
		var current Container
		if err := json.Unmarshal(data, &current); err != nil {
			return fmt.Errorf("unmarshal current container state: %w", err)
		}
		if current.Revision != c.Revision {
			return fmt.Errorf("%w for %s: caller=%d current=%d", ErrRevisionConflict, c.ID, c.Revision, current.Revision)
		}
		if current.Revision == math.MaxUint64 {
			return fmt.Errorf("container %s revision overflow", c.ID)
		}
		nextRevision = current.Revision + 1
	case errors.Is(err, os.ErrNotExist):
		if c.Revision != 0 {
			return fmt.Errorf("%w for %s: record was deleted after revision %d", ErrRevisionConflict, c.ID, c.Revision)
		}
	default:
		return fmt.Errorf("read current container state: %w", err)
	}

	return s.writeContainerRevisionUnlocked(c, nextRevision)
}

func (s *Store) writeContainerNextRevisionUnlocked(c *Container) error {
	if c.Revision == math.MaxUint64 {
		return fmt.Errorf("container %s revision overflow", c.ID)
	}
	return s.writeContainerRevisionUnlocked(c, c.Revision+1)
}

func (s *Store) writeContainerRevisionUnlocked(c *Container, revision uint64) error {
	copy := *c
	copy.Revision = revision
	data, err := json.MarshalIndent(&copy, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal container: %w", err)
	}
	target := filepath.Join(s.ctrDir, c.ID+".json")
	if err := atomicWriteFile(s.ctrDir, target, data); err != nil {
		return err
	}
	// Only publish the new revision to the caller after durable file creation
	// and rename succeeded. Failed writes leave the caller's CAS token intact.
	c.Revision = revision
	return nil
}
