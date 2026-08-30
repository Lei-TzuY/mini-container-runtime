package dns

import (
	"bytes"
	"encoding/json"
	"fmt"
)

const currentDNSRegistrySchemaVersion = 1

type dnsRegistryEnvelope struct {
	SchemaVersion int         `json:"schema_version"`
	NetworkName   string      `json:"network_name"`
	Entries       []HostEntry `json:"entries"`
}

// decodeDNSRegistry binds current registry contents to the network storage key.
// Bare arrays remain readable only as the historical pre-envelope format; every
// current write upgrades the registry to an explicit, versioned envelope.
func decodeDNSRegistry(data []byte, expectedNetworkName string) ([]HostEntry, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty DNS registry")
	}

	if trimmed[0] == '[' {
		var entries []HostEntry
		if err := json.Unmarshal(trimmed, &entries); err != nil {
			return nil, err
		}
		return entries, nil
	}
	if trimmed[0] != '{' {
		return nil, fmt.Errorf("DNS registry must be a versioned object or historical array")
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &raw); err != nil {
		return nil, err
	}
	rawVersion, ok := raw["schema_version"]
	if !ok {
		return nil, fmt.Errorf("DNS registry object missing schema version")
	}
	var version int
	if err := json.Unmarshal(rawVersion, &version); err != nil {
		return nil, fmt.Errorf("invalid DNS registry schema version: %w", err)
	}
	if version != currentDNSRegistrySchemaVersion {
		return nil, fmt.Errorf("unsupported DNS registry schema version %d", version)
	}

	rawNetwork, ok := raw["network_name"]
	if !ok {
		return nil, fmt.Errorf("DNS registry missing network provenance")
	}
	var networkName string
	if err := json.Unmarshal(rawNetwork, &networkName); err != nil {
		return nil, fmt.Errorf("invalid DNS registry network provenance: %w", err)
	}
	if networkName == "" || networkName != expectedNetworkName {
		return nil, fmt.Errorf("DNS registry network provenance %q does not match storage key %q", networkName, expectedNetworkName)
	}

	rawEntries, ok := raw["entries"]
	if !ok {
		return nil, fmt.Errorf("DNS registry missing entries")
	}
	var entries []HostEntry
	if err := json.Unmarshal(rawEntries, &entries); err != nil {
		return nil, fmt.Errorf("decode DNS registry entries: %w", err)
	}
	return entries, nil
}

func encodeDNSRegistry(networkName string, entries []HostEntry) ([]byte, error) {
	return json.MarshalIndent(dnsRegistryEnvelope{
		SchemaVersion: currentDNSRegistrySchemaVersion,
		NetworkName:   networkName,
		Entries:       entries,
	}, "", "  ")
}
