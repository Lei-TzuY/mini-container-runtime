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
	Root struct {
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
	Linux *struct {
		Namespaces []struct {
			Type string `json:"type"`
			Path string `json:"path,omitempty"`
		} `json:"namespaces,omitempty"`
	} `json:"linux,omitempty"`
}

func loadOCIBundle(bundle string) (container.Config, error) {
	abs, err := filepath.Abs(bundle)
	if err != nil { return container.Config{}, fmt.Errorf("resolve bundle: %w", err) }
	data, err := os.ReadFile(filepath.Join(abs, "config.json"))
	if err != nil { return container.Config{}, fmt.Errorf("read config.json: %w", err) }
	var spec ociBundleConfig
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&spec); err != nil { return container.Config{}, fmt.Errorf("decode config.json: %w", err) }
	if spec.OCIVersion == "" { return container.Config{}, fmt.Errorf("ociVersion is required") }
	if spec.Root.Path == "" { return container.Config{}, fmt.Errorf("root.path is required") }
	if filepath.IsAbs(spec.Root.Path) { return container.Config{}, fmt.Errorf("root.path must be relative to bundle") }
	if len(spec.Process.Args) == 0 || spec.Process.Args[0] == "" { return container.Config{}, fmt.Errorf("process.args must contain an executable") }
	if spec.Process.Cwd == "" || !filepath.IsAbs(spec.Process.Cwd) { return container.Config{}, fmt.Errorf("process.cwd must be absolute") }
	if spec.Process.Terminal { return container.Config{}, fmt.Errorf("process.terminal is not supported") }
	for _, e := range spec.Process.Env {
		if i := strings.IndexByte(e, '='); i <= 0 { return container.Config{}, fmt.Errorf("invalid process.env entry %q", e) }
	}
	if spec.Linux != nil {
		for _, ns := range spec.Linux.Namespaces {
			if ns.Path != "" { return container.Config{}, fmt.Errorf("joining existing %s namespace is not supported", ns.Type) }
			switch ns.Type {
			case "pid", "mount", "uts", "ipc", "network", "user", "cgroup":
			default: return container.Config{}, fmt.Errorf("unsupported linux namespace %q", ns.Type)
			}
		}
	}
	rootfs := filepath.Clean(filepath.Join(abs, spec.Root.Path))
	rel, err := filepath.Rel(abs, rootfs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) { return container.Config{}, fmt.Errorf("root.path escapes bundle") }
	return container.Config{RootFS: rootfs, ReadOnly: spec.Root.Readonly, Command: append([]string(nil), spec.Process.Args...), Env: append([]string(nil), spec.Process.Env...), WorkDir: spec.Process.Cwd, Hostname: spec.Hostname, UserNS: true}, nil
}
