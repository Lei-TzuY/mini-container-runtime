package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// RestartPortMapping is the durable form of one published container port.
type RestartPortMapping struct {
	HostPort      int    `json:"host_port"`
	ContainerPort int    `json:"container_port"`
	Protocol      string `json:"protocol,omitempty"`
}

// RestartVolume is the durable form of one bind or named-volume mount.
type RestartVolume struct {
	HostPath      string `json:"host_path"`
	ContainerPath string `json:"container_path"`
	ReadOnly      bool   `json:"read_only,omitempty"`
}

// RestartSpec is the durable execution input required to relaunch a stopped
// container without reconstructing runtime behavior from new CLI arguments.
// Security, isolation, resource, mount and network settings are persisted with
// the payload so restart cannot silently weaken the original execution policy.
type RestartSpec struct {
	RootFS           string               `json:"rootfs"`
	Command          []string             `json:"command"`
	Env              []string             `json:"env,omitempty"`
	WorkDir          string               `json:"work_dir,omitempty"`
	Hostname         string               `json:"hostname,omitempty"`
	Overlay          bool                 `json:"overlay,omitempty"`
	ReadOnly         bool                 `json:"read_only,omitempty"`
	Restart          string               `json:"restart,omitempty"`
	CapDrop          []string             `json:"cap_drop,omitempty"`
	NoNewPrivileges  bool                 `json:"no_new_privileges,omitempty"`
	ProcessUserSet   bool                 `json:"process_user_set,omitempty"`
	ProcessUID       uint32               `json:"process_uid,omitempty"`
	ProcessGID       uint32               `json:"process_gid,omitempty"`
	ProcessGroups    []uint32             `json:"process_groups,omitempty"`
	ProcessUmaskSet  bool                 `json:"process_umask_set,omitempty"`
	ProcessUmask     uint32               `json:"process_umask,omitempty"`
	Memory           int64                `json:"memory,omitempty"`
	CPUWeight        int64                `json:"cpu_weight,omitempty"`
	CPUs             float64              `json:"cpus,omitempty"`
	PidsLimit        int64                `json:"pids_limit,omitempty"`
	Seccomp          bool                 `json:"seccomp,omitempty"`
	BridgeNetwork    bool                 `json:"bridge_network,omitempty"`
	PortMappings     []RestartPortMapping `json:"port_mappings,omitempty"`
	Volumes          []RestartVolume      `json:"volumes,omitempty"`
	UserNS           bool                 `json:"user_ns"`
	CgroupNS         bool                 `json:"cgroup_ns,omitempty"`
	Debug            bool                 `json:"debug,omitempty"`
}

func restartSpecPath(dir, containerID string) string {
	return filepath.Join(dir, containerID+".restart")
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
	return atomicWriteFile(s.ctrDir, restartSpecPath(s.ctrDir, containerID), data)
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
	data, err := readRegularStateFile(restartSpecPath(s.ctrDir, containerID), "container restart spec")
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
	spec.CapDrop = append([]string(nil), spec.CapDrop...)
	spec.ProcessGroups = append([]uint32(nil), spec.ProcessGroups...)
	spec.PortMappings = append([]RestartPortMapping(nil), spec.PortMappings...)
	spec.Volumes = append([]RestartVolume(nil), spec.Volumes...)
	return spec, nil
}
