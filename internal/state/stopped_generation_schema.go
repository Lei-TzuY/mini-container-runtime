package state

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

const currentStoppedGenerationSchemaVersion uint32 = 1

// stoppedGenerationSchemaVersionUnlocked returns the explicitly persisted
// schema version for a modern stopped generation. Absence is reserved for
// upgrade compatibility with records written before schema versioning existed.
// Explicit zero and future versions are never interpreted as historical state.
func (s *Store) stoppedGenerationSchemaVersionUnlocked(id string) (version uint32, present bool, err error) {
	if err := validateID(id); err != nil {
		return 0, false, err
	}
	path := filepath.Join(s.ctrDir, id+".json")
	data, err := readRegularStateFile(path, "container state")
	if err != nil {
		return 0, false, err
	}
	var metadata struct {
		Version *uint32 `json:"stopped_generation_schema_version"`
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return 0, false, fmt.Errorf("unmarshal stopped generation schema version: %w", err)
	}
	if metadata.Version == nil {
		return 0, false, nil
	}
	if *metadata.Version == 0 {
		return 0, true, fmt.Errorf("invalid stopped generation schema version 0")
	}
	if *metadata.Version != currentStoppedGenerationSchemaVersion {
		return *metadata.Version, true, fmt.Errorf(
			"unsupported stopped generation schema version %d (current %d)",
			*metadata.Version,
			currentStoppedGenerationSchemaVersion,
		)
	}
	return *metadata.Version, true, nil
}
