package state

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

const currentStoppedGenerationSchemaVersion uint32 = 1

type stoppedGenerationTeardownSnapshot struct {
	status             Status
	revision           uint64
	version            uint32
	versioned          bool
	required           bool
	requirementPresent bool
	identity           exitedIdentity
	identityEmbedded   bool
}

type stoppedGenerationLifecycleSnapshot struct {
	status               Status
	revision             uint64
	version               json.RawMessage
	exitIdentityRequired  json.RawMessage
	exitIdentity          json.RawMessage
}

// readStoppedGenerationLifecycleSnapshotUnlocked reads the container lifecycle
// and teardown authority fields from exactly one container-state file snapshot.
// Teardown fields stay raw until the caller has established that status/revision
// still name the lifecycle generation it intends to act on.
func (s *Store) readStoppedGenerationLifecycleSnapshotUnlocked(id string) (stoppedGenerationLifecycleSnapshot, error) {
	if err := validateID(id); err != nil {
		return stoppedGenerationLifecycleSnapshot{}, err
	}
	path := filepath.Join(s.ctrDir, id+".json")
	data, err := readRegularStateFile(path, "container state")
	if err != nil {
		return stoppedGenerationLifecycleSnapshot{}, err
	}
	var metadata struct {
		Status               Status          `json:"status"`
		Revision             uint64          `json:"revision"`
		Version              json.RawMessage `json:"stopped_generation_schema_version"`
		ExitIdentityRequired json.RawMessage `json:"exit_identity_required"`
		ExitIdentity         json.RawMessage `json:"exit_identity"`
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return stoppedGenerationLifecycleSnapshot{}, fmt.Errorf("unmarshal stopped generation lifecycle snapshot: %w", err)
	}
	return stoppedGenerationLifecycleSnapshot{
		status:              metadata.Status,
		revision:            metadata.Revision,
		version:             metadata.Version,
		exitIdentityRequired: metadata.ExitIdentityRequired,
		exitIdentity:         metadata.ExitIdentity,
	}, nil
}

func (raw stoppedGenerationLifecycleSnapshot) teardownSnapshot() (stoppedGenerationTeardownSnapshot, error) {
	snapshot := stoppedGenerationTeardownSnapshot{
		status:   raw.status,
		revision: raw.revision,
	}

	if len(raw.version) != 0 {
		snapshot.versioned = true
		if err := json.Unmarshal(raw.version, &snapshot.version); err != nil {
			return snapshot, fmt.Errorf("unmarshal stopped generation schema version: %w", err)
		}
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

	if len(raw.exitIdentityRequired) != 0 {
		snapshot.requirementPresent = true
		if err := json.Unmarshal(raw.exitIdentityRequired, &snapshot.required); err != nil {
			return snapshot, fmt.Errorf("unmarshal persisted exit identity requirement: %w", err)
		}
		if !snapshot.required {
			return snapshot, fmt.Errorf("invalid persisted exit identity requirement: false")
		}
	}

	if len(raw.exitIdentity) != 0 {
		if string(raw.exitIdentity) == "null" {
			return snapshot, fmt.Errorf("invalid persisted exit identity: null")
		}
		if err := json.Unmarshal(raw.exitIdentity, &snapshot.identity); err != nil {
			return snapshot, fmt.Errorf("unmarshal embedded exited process identity: %w", err)
		}
		if err := validateExitedIdentity(snapshot.identity); err != nil {
			return snapshot, err
		}
		snapshot.identityEmbedded = true
	}
	return snapshot, nil
}

// readStoppedGenerationTeardownSnapshotUnlocked reads lifecycle coordinates,
// schema, capability, and exact identity from one container-state file snapshot.
func (s *Store) readStoppedGenerationTeardownSnapshotUnlocked(id string) (stoppedGenerationTeardownSnapshot, error) {
	raw, err := s.readStoppedGenerationLifecycleSnapshotUnlocked(id)
	if err != nil {
		return stoppedGenerationTeardownSnapshot{}, err
	}
	return raw.teardownSnapshot()
}

// readStoppedGenerationTeardownSnapshotForRevisionUnlocked first checks the
// caller's stopped revision against the same JSON snapshot that carries teardown
// authority. Stale callers return current=false without interpreting potentially
// malformed authority metadata from a newer generation.
func (s *Store) readStoppedGenerationTeardownSnapshotForRevisionUnlocked(id string, revision uint64) (snapshot stoppedGenerationTeardownSnapshot, current bool, err error) {
	raw, err := s.readStoppedGenerationLifecycleSnapshotUnlocked(id)
	if err != nil {
		return stoppedGenerationTeardownSnapshot{}, false, err
	}
	if raw.status != StatusStopped || raw.revision != revision {
		return stoppedGenerationTeardownSnapshot{status: raw.status, revision: raw.revision}, false, nil
	}
	snapshot, err = raw.teardownSnapshot()
	if err != nil {
		return snapshot, true, err
	}
	return snapshot, true, nil
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
