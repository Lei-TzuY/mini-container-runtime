package state

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

const currentStoppedGenerationSchemaVersion uint32 = 1

type stoppedGenerationTeardownSnapshot struct {
	version            uint32
	versioned          bool
	required           bool
	requirementPresent bool
	identity           exitedIdentity
	identityEmbedded   bool
}

// readStoppedGenerationTeardownSnapshotUnlocked reads and parses the lifecycle
// teardown metadata from one container-state file snapshot. Callers that need
// schema, capability, and exact identity together must use this helper instead
// of independently rereading the JSON and risking a mixed snapshot.
func (s *Store) readStoppedGenerationTeardownSnapshotUnlocked(id string) (stoppedGenerationTeardownSnapshot, error) {
	if err := validateID(id); err != nil {
		return stoppedGenerationTeardownSnapshot{}, err
	}
	path := filepath.Join(s.ctrDir, id+".json")
	data, err := readRegularStateFile(path, "container state")
	if err != nil {
		return stoppedGenerationTeardownSnapshot{}, err
	}
	var metadata struct {
		Version              *uint32         `json:"stopped_generation_schema_version"`
		ExitIdentityRequired *bool           `json:"exit_identity_required"`
		ExitIdentity         json.RawMessage `json:"exit_identity"`
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return stoppedGenerationTeardownSnapshot{}, fmt.Errorf("unmarshal stopped generation teardown metadata: %w", err)
	}

	var snapshot stoppedGenerationTeardownSnapshot
	if metadata.Version != nil {
		snapshot.versioned = true
		snapshot.version = *metadata.Version
		if snapshot.version == 0 {
			return snapshot, fmt.Errorf("invalid stopped generation schema version 0")
		}
		if snapshot.version != currentStoppedGenerationSchemaVersion {
			return snapshot, fmt.Errorf(
				"unsupported stopped generation schema version %d (current %d)",
				snapshot.version,
				currentStoppedGenerationSchemaVersion,
			)
		}
	}

	if metadata.ExitIdentityRequired != nil {
		snapshot.requirementPresent = true
		if !*metadata.ExitIdentityRequired {
			return snapshot, fmt.Errorf("invalid persisted exit identity requirement: false")
		}
		snapshot.required = true
	}

	if len(metadata.ExitIdentity) != 0 {
		if string(metadata.ExitIdentity) == "null" {
			return snapshot, fmt.Errorf("invalid persisted exit identity: null")
		}
		if err := json.Unmarshal(metadata.ExitIdentity, &snapshot.identity); err != nil {
			return snapshot, fmt.Errorf("unmarshal embedded exited process identity: %w", err)
		}
		if err := validateExitedIdentity(snapshot.identity); err != nil {
			return snapshot, err
		}
		snapshot.identityEmbedded = true
	}
	return snapshot, nil
}

// stoppedGenerationSchemaVersionUnlocked returns the explicitly persisted
// schema version for a modern stopped generation. Absence is reserved for
// upgrade compatibility with records written before schema versioning existed.
// Explicit zero and future versions are never interpreted as historical state.
func (s *Store) stoppedGenerationSchemaVersionUnlocked(id string) (version uint32, present bool, err error) {
	snapshot, err := s.readStoppedGenerationTeardownSnapshotUnlocked(id)
	if err != nil {
		return snapshot.version, snapshot.versioned, err
	}
	return snapshot.version, snapshot.versioned, nil
}
