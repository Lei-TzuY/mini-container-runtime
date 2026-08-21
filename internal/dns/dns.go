package dns

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"minicontainer/internal/state"
)

var (
	dnsMu                 sync.Mutex
	validNetworkNameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)
	validHostnameRegex    = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`)
)

type HostEntry struct {
	ContainerID string `json:"container_id"`
	Hostname    string `json:"hostname"`
	IP          string `json:"ip"`
}

type NetworkDNS struct {
	mu  sync.Mutex
	dir string
}

func DefaultDNSDir() string {
	return filepath.Join(state.DefaultDir(), "dns")
}

func validateNetworkName(name string) error {
	if name == "" {
		return fmt.Errorf("network name cannot be empty")
	}
	if name == "." || name == ".." || strings.ContainsAny(name, "/\\:") {
		return fmt.Errorf("invalid network name %q: path separators and relative components not allowed", name)
	}
	if !validNetworkNameRegex.MatchString(name) {
		return fmt.Errorf("invalid network name %q: must start with alphanumeric character and contain only [a-zA-Z0-9_.-]", name)
	}
	return nil
}

func validateHostAndIP(hostname, ipAddr string) error {
	if hostname == "" {
		return fmt.Errorf("hostname cannot be empty")
	}
	if strings.ContainsAny(hostname, " \t\r\n\x00") || !validHostnameRegex.MatchString(hostname) {
		return fmt.Errorf("invalid hostname %q: must be a valid DNS name without whitespace or control characters", hostname)
	}
	if ipAddr == "" {
		return fmt.Errorf("IP address cannot be empty")
	}
	if net.ParseIP(ipAddr) == nil {
		return fmt.Errorf("invalid IP address %q", ipAddr)
	}
	return nil
}

// RegisterHost records a container IP mapping in a network.
func RegisterHost(networkName, containerID, hostname, ipAddr string) error {
	if err := validateNetworkName(networkName); err != nil {
		return err
	}
	if containerID == "" {
		return fmt.Errorf("container ID cannot be empty")
	}
	if err := validateHostAndIP(hostname, ipAddr); err != nil {
		return err
	}

	dnsMu.Lock()
	defer dnsMu.Unlock()

	dir := DefaultDNSDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	netFile := filepath.Join(dir, networkName+".json")
	entries := loadEntries(netFile)

	// Filter out old entry for same containerID or hostname
	var updated []HostEntry
	for _, e := range entries {
		if e.ContainerID != containerID && e.Hostname != hostname {
			updated = append(updated, e)
		}
	}

	updated = append(updated, HostEntry{
		ContainerID: containerID,
		Hostname:    hostname,
		IP:          ipAddr,
	})

	return saveEntries(netFile, updated)
}

// UnregisterHost removes a container host entry.
func UnregisterHost(networkName, containerID string) error {
	if err := validateNetworkName(networkName); err != nil {
		return err
	}

	dnsMu.Lock()
	defer dnsMu.Unlock()

	netFile := filepath.Join(DefaultDNSDir(), networkName+".json")
	entries := loadEntries(netFile)

	var updated []HostEntry
	for _, e := range entries {
		if e.ContainerID != containerID {
			updated = append(updated, e)
		}
	}

	return saveEntries(netFile, updated)
}

// GenerateHostsContent formats hosts mapping lines.
func GenerateHostsContent(networkName string) string {
	if err := validateNetworkName(networkName); err != nil {
		return ""
	}

	dnsMu.Lock()
	defer dnsMu.Unlock()

	netFile := filepath.Join(DefaultDNSDir(), networkName+".json")
	entries := loadEntries(netFile)

	var lines []string
	lines = append(lines, "127.0.0.1\tlocalhost")
	lines = append(lines, "::1\tlocalhost ip6-localhost ip6-loopback")
	lines = append(lines, "# Mini Docker Network Service Discovery ("+networkName+")")

	for _, e := range entries {
		if e.IP != "" && e.Hostname != "" {
			lines = append(lines, fmt.Sprintf("%s\t%s", e.IP, e.Hostname))
		}
	}

	return strings.Join(lines, "\n") + "\n"
}

// InjectHostsIntoRootFS writes updated /etc/hosts file inside container rootfs.
func InjectHostsIntoRootFS(rootfsPath, networkName string) error {
	if err := validateNetworkName(networkName); err != nil {
		return err
	}

	content := GenerateHostsContent(networkName)
	etcDir := filepath.Join(rootfsPath, "etc")
	if err := os.MkdirAll(etcDir, 0755); err != nil {
		return err
	}

	hostsFile := filepath.Join(etcDir, "hosts")
	return os.WriteFile(hostsFile, []byte(content), 0644)
}

func loadEntries(path string) []HostEntry {
	data, err := os.ReadFile(path)
	if err != nil {
		return []HostEntry{}
	}
	var entries []HostEntry
	_ = json.Unmarshal(data, &entries)
	return entries
}

func saveEntries(path string, entries []HostEntry) error {
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
