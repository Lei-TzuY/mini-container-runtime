package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"minicontainer/internal/container"
)

func promoteOCIMaskedDirectories(bundle string, cfg *container.Config) error {
	if cfg == nil {
		return fmt.Errorf("OCI masked directory config is nil")
	}
	data, err := os.ReadFile(filepath.Join(bundle, "config.json"))
	if err != nil {
		return fmt.Errorf("read OCI masked directory policy: %w", err)
	}
	var policy struct {
		Linux *struct {
			MaskedPaths []string `json:"maskedPaths,omitempty"`
		} `json:"linux,omitempty"`
	}
	if err := json.Unmarshal(data, &policy); err != nil {
		return fmt.Errorf("decode OCI masked directory policy: %w", err)
	}
	if policy.Linux == nil || len(policy.Linux.MaskedPaths) == 0 {
		return nil
	}

	for _, containerPath := range policy.Linux.MaskedPaths {
		hostTarget := filepath.Join(cfg.RootFS, strings.TrimPrefix(containerPath, "/"))
		info, err := os.Stat(hostTarget)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("stat OCI masked path %q: %w", containerPath, err)
		}
		if !info.IsDir() {
			continue
		}

		index := -1
		for i := len(cfg.Volumes) - 1; i >= 0; i-- {
			volume := cfg.Volumes[i]
			if volume.HostPath == "/dev/null" && volume.ContainerPath == containerPath && volume.ReadOnly {
				index = i
				break
			}
		}
		if index < 0 {
			return fmt.Errorf("OCI masked directory %q has no admitted /dev/null policy to replace", containerPath)
		}
		cfg.Volumes = append(cfg.Volumes[:index], cfg.Volumes[index+1:]...)
		cfg.MaskedDirectories = append(cfg.MaskedDirectories, containerPath)
	}
	return nil
}
