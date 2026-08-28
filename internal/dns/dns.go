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
	ContainerID         string `json:"container_id"`
	Hostname            string `json:"hostname"`
	IP                  string `json:"ip"`
	OwnerPID            int    `json:"owner_pid,omitempty"`
	OwnerStartTime      uint64 `json:"owner_start_time,omitempty"`
	GenerationPID       int    `json:"generation_pid,omitempty"`
	GenerationStartTime uint64 `json:"generation_start_time,omitempty"`
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

func validateHostEntryOwner(entry HostEntry) error {
	legacy := entry.OwnerPID == 0 && entry.OwnerStartTime == 0
	if !legacy && (entry.OwnerPID <= 0 || entry.OwnerStartTime == 0) {
		return fmt.Errorf("incomplete registrar process identity %d/%d", entry.OwnerPID, entry.OwnerStartTime)
	}
	generationUnset := entry.GenerationPID == 0 && entry.GenerationStartTime == 0
	if !generationUnset && (entry.GenerationPID <= 0 || entry.GenerationStartTime == 0) {
		return fmt.Errorf("incomplete child process identity %d/%d", entry.GenerationPID, entry.GenerationStartTime)
	}
	if legacy && !generationUnset {
		return fmt.Errorf("child process identity requires registrar ownership")
	}
	return nil
}

func validateEntries(networkName string, entries []HostEntry) error {
	seenContainers := make(map[string]struct{}, len(entries))
	seenHostnames := make(map[string]struct{}, len(entries))
	for i, entry := range entries {
		if strings.TrimSpace(entry.ContainerID) == "" {
			return fmt.Errorf("DNS registry %q entry %d has empty container ID", networkName, i)
		}
		if err := validateHostAndIP(entry.Hostname, entry.IP); err != nil {
			return fmt.Errorf("DNS registry %q entry %d is invalid: %w", networkName, i, err)
		}
		if err := validateHostEntryOwner(entry); err != nil {
			return fmt.Errorf("DNS registry %q entry %d has invalid ownership: %w", networkName, i, err)
		}
		if _, ok := seenContainers[entry.ContainerID]; ok {
			return fmt.Errorf("DNS registry %q has duplicate container ID %q", networkName, entry.ContainerID)
		}
		if _, ok := seenHostnames[entry.Hostname]; ok {
			return fmt.Errorf("DNS registry %q has duplicate hostname %q", networkName, entry.Hostname)
		}
		seenContainers[entry.ContainerID] = struct{}{}
		seenHostnames[entry.Hostname] = struct{}{}
	}
	return nil
}

// pruneStaleOwnedEntries removes only entries whose registrar process generation
// is authoritatively gone and whose container generation is not still running.
// Legacy entries lack generation proof and are deliberately retained rather
// than guessed stale.
func pruneStaleOwnedEntries(entries []HostEntry) ([]HostEntry, bool, error) {
	if len(entries) == 0 {
		return entries, false, nil
	}
	kept := make([]HostEntry, 0, len(entries))
	changed := false
	for _, entry := range entries {
		if entry.OwnerPID == 0 && entry.OwnerStartTime == 0 {
			kept = append(kept, entry)
			continue
		}
		active, err := hostEntryOwnerActive(entry)
		if err != nil {
			return nil, false, fmt.Errorf("resolve DNS ownership for container %s: %w", entry.ContainerID, err)
		}
		if active {
			kept = append(kept, entry)
			continue
		}
		changed = true
	}
	return kept, changed, nil
}

func ensureDNSDir() (string, error) {
	dir := DefaultDNSDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create DNS registry directory %q: %w", dir, err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return "", fmt.Errorf("inspect DNS registry directory %q: %w", dir, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", fmt.Errorf("DNS registry path %q must be a real directory", dir)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return "", fmt.Errorf("chmod DNS registry directory %q: %w", dir, err)
	}
	return dir, nil
}

func loadEntriesChecked(path, networkName string) ([]HostEntry, bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("inspect DNS registry %q: %w", networkName, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, false, fmt.Errorf("DNS registry %q must be a regular file", networkName)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false, fmt.Errorf("read DNS registry %q: %w", networkName, err)
	}
	var entries []HostEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, false, fmt.Errorf("parse DNS registry %q: %w", networkName, err)
	}
	if err := validateEntries(networkName, entries); err != nil {
		return nil, false, err
	}
	return entries, true, nil
}

func saveEntriesAtomic(dir, path, networkName string, entries []HostEntry) error {
	if err := validateEntries(networkName, entries); err != nil {
		return err
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal DNS registry %q: %w", networkName, err)
	}

	tmp, err := os.CreateTemp(dir, "."+networkName+".json.tmp-*")
	if err != nil {
		return fmt.Errorf("create DNS registry temp file %q: %w", networkName, err)
	}
	tmpName := tmp.Name()
	closed := false
	defer func() {
		if !closed {
			_ = tmp.Close()
		}
		_ = os.Remove(tmpName)
	}()

	if err := tmp.Chmod(0o600); err != nil {
		return fmt.Errorf("chmod DNS registry temp file %q: %w", networkName, err)
	}
	n, err := tmp.Write(data)
	if err != nil {
		return fmt.Errorf("write DNS registry temp file %q: %w", networkName, err)
	}
	if n != len(data) {
		return fmt.Errorf("write DNS registry temp file %q: short write %d/%d", networkName, n, len(data))
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync DNS registry temp file %q: %w", networkName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close DNS registry temp file %q: %w", networkName, err)
	}
	closed = true
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("publish DNS registry %q: %w", networkName, err)
	}

	dirFile, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open DNS registry directory for sync: %w", err)
	}
	defer dirFile.Close()
	if err := dirFile.Sync(); err != nil {
		return fmt.Errorf("sync DNS registry directory: %w", err)
	}
	return nil
}

func entriesWithRegistration(entries []HostEntry, owner registrarIdentity, containerID, hostname, ipAddr string) ([]HostEntry, bool, error) {
	for _, entry := range entries {
		if entry.ContainerID != containerID && entry.Hostname != hostname {
			continue
		}
		if entry.ContainerID == containerID && entry.Hostname == hostname && entry.IP == ipAddr && entry.OwnerPID == owner.PID && entry.OwnerStartTime == owner.StartTime {
			return entries, false, nil
		}
		return nil, false, fmt.Errorf(
			"live DNS registration conflict: container %q/hostname %q is owned by registrar %d/%d",
			entry.ContainerID,
			entry.Hostname,
			entry.OwnerPID,
			entry.OwnerStartTime,
		)
	}
	updated := append([]HostEntry(nil), entries...)
	updated = append(updated, HostEntry{
		ContainerID:    containerID,
		Hostname:       hostname,
		IP:             ipAddr,
		OwnerPID:       owner.PID,
		OwnerStartTime: owner.StartTime,
	})
	return updated, true, nil
}

// RegisterHost records a container IP mapping in a network. The registration is
// owned by the exact minictl parent process generation so an os.Exit, kill, or
// crash cannot leave it permanently authoritative. If the registrar disappears
// while its container remains alive, the committed child generation adopts the
// entry until that generation exits. A still-live registration owned by another
// registrar is never overwritten: competing lifecycle actors must fail closed
// rather than steal service-discovery ownership from the generation that won.
func RegisterHost(networkName, containerID, hostname, ipAddr string) error {
	if err := validateNetworkName(networkName); err != nil {
		return err
	}
	if strings.TrimSpace(containerID) == "" {
		return fmt.Errorf("container ID cannot be empty")
	}
	if err := validateHostAndIP(hostname, ipAddr); err != nil {
		return err
	}
	owner, err := currentRegistrarIdentity()
	if err != nil {
		return err
	}

	dnsMu.Lock()
	defer dnsMu.Unlock()
	dir, err := ensureDNSDir()
	if err != nil {
		return err
	}
	return withDNSNetworkLock(dir, networkName, func() error {
		netFile := filepath.Join(dir, networkName+".json")
		entries, _, err := loadEntriesChecked(netFile, networkName)
		if err != nil {
			return err
		}
		entries, pruned, err := pruneStaleOwnedEntries(entries)
		if err != nil {
			return err
		}
		updated, changed, err := entriesWithRegistration(entries, owner, containerID, hostname, ipAddr)
		if err != nil {
			return err
		}
		if !changed && !pruned {
			return nil
		}
		return saveEntriesAtomic(dir, netFile, networkName, updated)
	})
}

// UnregisterHost is the compatibility teardown API. Modern registrations are
// generation-owned, so an unscoped container-ID delete is unsafe: a stale CLI
// defer could otherwise remove a replacement entry published by a newer runtime.
// Preserve historical call sites while giving them the same exact registrar
// ownership boundary as authoritative runtime finalization.
func UnregisterHost(networkName, containerID string) error {
	return UnregisterHostOwned(networkName, containerID)
}

// GenerateHostsContentChecked returns a consistent snapshot of one registry.
// Dead process-owned entries are garbage-collected transactionally before the
// snapshot is formatted. Corrupt, symlinked, unreadable, or unprobeable state is
// reported so runtime setup can fail closed instead of using stale discovery.
func GenerateHostsContentChecked(networkName string) (string, error) {
	if err := validateNetworkName(networkName); err != nil {
		return "", err
	}

	dnsMu.Lock()
	defer dnsMu.Unlock()
	dir, err := ensureDNSDir()
	if err != nil {
		return "", err
	}
	var entries []HostEntry
	if err := withDNSNetworkLock(dir, networkName, func() error {
		netFile := filepath.Join(dir, networkName+".json")
		var exists bool
		var loadErr error
		entries, exists, loadErr = loadEntriesChecked(netFile, networkName)
		if loadErr != nil || !exists {
			return loadErr
		}
		var changed bool
		entries, changed, loadErr = pruneStaleOwnedEntries(entries)
		if loadErr != nil {
			return loadErr
		}
		if changed {
			return saveEntriesAtomic(dir, netFile, networkName, entries)
		}
		return nil
	}); err != nil {
		return "", err
	}

	lines := []string{
		"127.0.0.1\tlocalhost",
		"::1\tlocalhost ip6-localhost ip6-loopback",
		"# Mini Docker Network Service Discovery (" + networkName + ")",
	}
	for _, entry := range entries {
		lines = append(lines, fmt.Sprintf("%s\t%s", entry.IP, entry.Hostname))
	}
	return strings.Join(lines, "\n") + "\n", nil
}

// GenerateHostsContent preserves the historical string-only API. New runtime
// paths should use GenerateHostsContentChecked so storage failures are visible.
func GenerateHostsContent(networkName string) string {
	content, err := GenerateHostsContentChecked(networkName)
	if err != nil {
		return ""
	}
	return content
}

// InjectHostsIntoRootFS is retained for API compatibility, but direct rootfs
// mutation is intentionally disabled. Container runs now bind-mount an
// anonymous generated hosts file inside the child mount namespace instead.
func InjectHostsIntoRootFS(rootfsPath, networkName string) error {
	if rootfsPath == "" {
		return fmt.Errorf("rootfs path cannot be empty")
	}
	if err := validateNetworkName(networkName); err != nil {
		return err
	}
	rootfsAbs, err := filepath.Abs(rootfsPath)
	if err != nil {
		return fmt.Errorf("resolve rootfs path %q: %w", rootfsPath, err)
	}
	st, err := os.Stat(rootfsAbs)
	if err != nil {
		return fmt.Errorf("stat rootfs %q: %w", rootfsPath, err)
	}
	if !st.IsDir() {
		return fmt.Errorf("rootfs %q is not a directory", rootfsPath)
	}
	return nil
}

func isSubDir(base, target string) bool {
	baseAbs, err1 := filepath.Abs(base)
	targetAbs, err2 := filepath.Abs(target)
	if err1 != nil || err2 != nil {
		return false
	}
	rel, err := filepath.Rel(baseAbs, targetAbs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}
