package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// RestartSpec is the durable execution input required to relaunch a stopped
// container without reconstructing runtime behavior from CLI arguments.
type RestartSpec struct {
	RootFS   string   `json:"rootfs"`
	Command  []string `json:"command"`
	Env      []string `json:"env,omitempty"`
	WorkDir  string   `json:"work_dir,omitempty"`
	Hostname string   `json:"hostname,omitempty"`
}

func (s *Store) SaveRestartSpec(containerID string, spec RestartSpec) error {
	if err := validateID(containerID); err != nil {
		return err
	}
	if spec.RootFS == "" {
		return fmt.Errorf("restart spec rootfs is empty")
	}
	if len(spec.Command) == 0 {
		return fmt.Errorf("restart spec command is empty")
	}
	data, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal restart spec: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := lockStateFile(s.lockFile); err != nil {
		return err
	}
	defer func() { _ = unlockStateFile(s.lockFile) }()
	if _, err := s.getUnlocked(containerID); err != nil {
		return fmt.Errorf("load restart spec owner: %w", err)
	}
	return atomicWriteFile(s.ctrDir, filepath.Join(s.ctrDir, containerID+".restart.json"), data)
}

func (s *Store) RestartSpec(containerID string) (RestartSpec, error) {
	if err := validateID(containerID); err != nil {
		return RestartSpec{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lockFile == nil {
		return RestartSpec{}, ErrStoreClosed
	}
	if _, err := s.getUnlocked(containerID); err != nil {
		return RestartSpec{}, fmt.Errorf("load restart spec owner: %w", err)
	}
	data, err := readRegularStateFile(filepath.Join(s.ctrDir, containerID+".restart.json"), "container restart spec")
	if err != nil {
		if os.IsNotExist(err) {
			return RestartSpec{}, fmt.Errorf("container %s has no durable restart spec: %w", containerID, err)
		}
		return RestartSpec{}, fmt.Errorf("read restart spec: %w", err)
	}
	var spec RestartSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return RestartSpec{}, fmt.Errorf("unmarshal restart spec: %w", err)
	}
	if spec.RootFS == "" || len(spec.Command) == 0 {
		return RestartSpec{}, fmt.Errorf("container %s restart spec is incomplete", containerID)
	}
	spec.Command = append([]string(nil), spec.Command...)
	spec.Env = append([]string(nil), spec.Env...)
	return spec, nil
}
