package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"

	"minicontainer/internal/state"
)

type PluginType string

const (
	PluginTypeVolume  PluginType = "volume"
	PluginTypeNetwork PluginType = "network"
	PluginTypeLog     PluginType = "log"
)

type Plugin struct {
	Name        string     `json:"name"`
	Version     string     `json:"version"`
	Type        PluginType `json:"type"`
	Executable  string     `json:"executable"`
	Description string     `json:"description,omitempty"`
	Enabled     bool       `json:"enabled"`
}

func PluginsDir() string {
	return filepath.Join(state.DefaultDir(), "plugins")
}

// ListPlugins reads all plugin manifests from ~/.minicontainer/plugins/.
func ListPlugins() ([]Plugin, error) {
	dir := PluginsDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var plugins []Plugin
	for _, entry := range entries {
		if entry.IsDir() {
			manifestPath := filepath.Join(dir, entry.Name(), "plugin.json")
			data, err := os.ReadFile(manifestPath)
			if err == nil {
				var p Plugin
				if err := json.Unmarshal(data, &p); err == nil {
					plugins = append(plugins, p)
				}
			}
		}
	}
	return plugins, nil
}

// InstallPlugin creates a new plugin manifest under ~/.minicontainer/plugins/<name>/.
func InstallPlugin(name, version string, pType PluginType, execPath string, desc string) error {
	pDir := filepath.Join(PluginsDir(), name)
	if err := os.MkdirAll(pDir, 0755); err != nil {
		return err
	}

	p := Plugin{
		Name:        name,
		Version:     version,
		Type:        pType,
		Executable:  execPath,
		Description: desc,
		Enabled:     true,
	}

	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(pDir, "plugin.json"), data, 0644)
}

// RemovePlugin deletes a plugin directory.
func RemovePlugin(name string) error {
	pDir := filepath.Join(PluginsDir(), name)
	return os.RemoveAll(pDir)
}
