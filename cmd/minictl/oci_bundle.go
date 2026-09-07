package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"minicontainer/internal/container"
)

type ociBundleConfig struct {
	OCIVersion string `json:"ociVersion"`
	Root       struct {
		Path     string `json:"path"`
		Readonly bool   `json:"readonly,omitempty"`
	} `json:"root"`
	Process struct {
		Terminal bool     `json:"terminal,omitempty"`
		Args     []string `json:"args"`
		Env      []string `json:"env,omitempty"`
		Cwd      string   `json:"cwd"`
	} `json:"process"`
	Hostname string `json:"hostname,omitempty"`
	Mounts []struct {
		Destination string   `json:"destination"`
		Type        string   `json:"type"`
		Source      string   `json:"source"`
		Options     []string `json:"options,omitempty"`
	} `json:"mounts,omitempty"`
	Linux *struct {
		Namespaces []struct {
			Type string `json:"type"`
			Path string `json:"path,omitempty"`
		} `json:"namespaces,omitempty"`
		Resources *struct {
			Memory *struct {
				Limit *int64 `json:"limit,omitempty"`
			} `json:"memory,omitempty"`
			CPU *struct {
				Shares *uint64 `json:"shares,omitempty"`
				Quota  *int64  `json:"quota,omitempty"`
				Period *uint64 `json:"period,omitempty"`
			} `json:"cpu,omitempty"`
			Pids *struct {
				Limit int64 `json:"limit"`
			} `json:"pids,omitempty"`
		} `json:"resources,omitempty"`
	} `json:"linux,omitempty"`
}

func loadOCIBundle(bundle string) (container.Config, error) {
	abs, err := filepath.Abs(bundle)
	if err != nil {
		return container.Config{}, fmt.Errorf("resolve bundle: %w", err)
	}
	data, err := os.ReadFile(filepath.Join(abs, "config.json"))
	if err != nil {
		return container.Config{}, fmt.Errorf("read config.json: %w", err)
	}
	var spec ociBundleConfig
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&spec); err != nil {
		return container.Config{}, fmt.Errorf("decode config.json: %w", err)
	}
	if spec.OCIVersion == "" {
		return container.Config{}, fmt.Errorf("ociVersion is required")
	}
	if spec.Root.Path == "" {
		return container.Config{}, fmt.Errorf("root.path is required")
	}
	if filepath.IsAbs(spec.Root.Path) {
		return container.Config{}, fmt.Errorf("root.path must be relative to bundle")
	}
	if len(spec.Process.Args) == 0 || spec.Process.Args[0] == "" {
		return container.Config{}, fmt.Errorf("process.args must contain an executable")
	}
	if spec.Process.Cwd == "" || !filepath.IsAbs(spec.Process.Cwd) {
		return container.Config{}, fmt.Errorf("process.cwd must be absolute")
	}
	if spec.Process.Terminal {
		return container.Config{}, fmt.Errorf("process.terminal is not supported")
	}
	for _, e := range spec.Process.Env {
		if i := strings.IndexByte(e, '='); i <= 0 {
			return container.Config{}, fmt.Errorf("invalid process.env entry %q", e)
		}
	}

	cfg := container.Config{
		ReadOnly: spec.Root.Readonly,
		Command:  append([]string(nil), spec.Process.Args...),
		Env:      append([]string(nil), spec.Process.Env...),
		WorkDir:  spec.Process.Cwd,
		Hostname: spec.Hostname,
	}
	for _, mount := range spec.Mounts {
		volume, err := translateOCIBindMount(mount.Destination, mount.Type, mount.Source, mount.Options)
		if err != nil {
			return container.Config{}, err
		}
		cfg.Volumes = append(cfg.Volumes, volume)
	}
	if spec.Linux != nil {
		seenNamespaces := make(map[string]struct{}, len(spec.Linux.Namespaces))
		for _, ns := range spec.Linux.Namespaces {
			if ns.Path != "" {
				return container.Config{}, fmt.Errorf("joining existing %s namespace is not supported", ns.Type)
			}
			if _, exists := seenNamespaces[ns.Type]; exists {
				return container.Config{}, fmt.Errorf("duplicate linux namespace %q", ns.Type)
			}
			seenNamespaces[ns.Type] = struct{}{}
			switch ns.Type {
			case "pid", "mount", "uts", "ipc", "network":
			case "user":
				cfg.UserNS = true
			case "cgroup":
				return container.Config{}, fmt.Errorf("linux cgroup namespace is not supported")
			default:
				return container.Config{}, fmt.Errorf("unsupported linux namespace %q", ns.Type)
			}
		}
		if err := applyOCIResources(&cfg, spec.Linux.Resources); err != nil {
			return container.Config{}, err
		}
	}

	rootfs := filepath.Clean(filepath.Join(abs, spec.Root.Path))
	rel, err := filepath.Rel(abs, rootfs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return container.Config{}, fmt.Errorf("root.path escapes bundle")
	}
	cfg.RootFS = rootfs
	return cfg, nil
}

func translateOCIBindMount(destination, mountType, source string, options []string) (container.Volume, error) {
	if mountType != "bind" {
		return container.Volume{}, fmt.Errorf("unsupported OCI mount type %q", mountType)
	}
	if !filepath.IsAbs(source) {
		return container.Volume{}, fmt.Errorf("OCI bind mount source %q must be absolute", source)
	}
	if !filepath.IsAbs(destination) || filepath.Clean(destination) == "/" {
		return container.Volume{}, fmt.Errorf("OCI bind mount destination %q must be an absolute path below root", destination)
	}
	readonly := false
	for _, option := range options {
		switch option {
		case "bind", "rbind", "rw":
		case "ro":
			readonly = true
		default:
			return container.Volume{}, fmt.Errorf("unsupported OCI bind mount option %q", option)
		}
	}
	return container.Volume{HostPath: filepath.Clean(source), ContainerPath: filepath.Clean(destination), ReadOnly: readonly}, nil
}

func applyOCIResources(cfg *container.Config, resources *struct {
	Memory *struct {
		Limit *int64 `json:"limit,omitempty"`
	} `json:"memory,omitempty"`
	CPU *struct {
		Shares *uint64 `json:"shares,omitempty"`
		Quota  *int64  `json:"quota,omitempty"`
		Period *uint64 `json:"period,omitempty"`
	} `json:"cpu,omitempty"`
	Pids *struct {
		Limit int64 `json:"limit"`
	} `json:"pids,omitempty"`
}) error {
	if resources == nil {
		return nil
	}
	if resources.Memory != nil && resources.Memory.Limit != nil {
		if *resources.Memory.Limit <= 0 {
			return fmt.Errorf("linux.resources.memory.limit must be greater than zero")
		}
		cfg.Memory = *resources.Memory.Limit
	}
	if resources.Pids != nil {
		if resources.Pids.Limit <= 0 {
			return fmt.Errorf("linux.resources.pids.limit must be greater than zero")
		}
		cfg.PidsLimit = resources.Pids.Limit
	}
	if resources.CPU != nil {
		cpu := resources.CPU
		if cpu.Shares != nil {
			if *cpu.Shares < 2 || *cpu.Shares > 262144 {
				return fmt.Errorf("linux.resources.cpu.shares must be in range 2..262144")
			}
			cfg.CPUWeight = 1 + int64((*cpu.Shares-2)*9999/262142)
		}
		if (cpu.Quota == nil) != (cpu.Period == nil) {
			return fmt.Errorf("linux.resources.cpu.quota and period must be specified together")
		}
		if cpu.Quota != nil {
			if *cpu.Quota == -1 {
				cfg.CPUs = 0
			} else {
				if *cpu.Quota <= 0 || *cpu.Period == 0 {
					return fmt.Errorf("linux.resources.cpu quota and period must be positive, or quota may be -1")
				}
				cfg.CPUs = float64(*cpu.Quota) / float64(*cpu.Period)
			}
		}
	}
	return nil
}
