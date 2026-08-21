// Package imagestore provides OCI image configuration inspection utilities.
// This file implements user mapping validation for rootless user namespaces (userns),
// evaluating numeric and symbolic UID/GID definitions in Image Configs.

package imagestore

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// UserNamespaceConfig contains parsed user execution details and userns mapping status.
type UserNamespaceConfig struct {
	RawUser           string
	UID               int
	GID               int
	IsRoot            bool
	IsNumeric         bool
	IsRootlessAllowed bool
}

// SubIDRange represents a range in /etc/subuid or /etc/subgid.
type SubIDRange struct {
	StartID uint32
	Length  uint32
}

// ValidateUserNamespaceMapping evaluates if the container image user can be mapped
// into a rootless user namespace range.
func ValidateUserNamespaceMapping(configJSON []byte, hostSubUIDRange SubIDRange) (UserNamespaceConfig, error) {
	var cfg struct {
		Config struct {
			User string `json:"User,omitempty"`
		} `json:"config"`
	}
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		return UserNamespaceConfig{}, fmt.Errorf("parse image config for user mapping: %w", err)
	}

	raw := strings.TrimSpace(cfg.Config.User)
	res := UserNamespaceConfig{
		RawUser: raw,
		UID:     0,
		GID:     0,
		IsRoot:  false,
	}

	if raw == "" || raw == "root" || raw == "0" || raw == "0:0" {
		res.IsRoot = true
		res.IsNumeric = true
		// Root in container maps to first host subuid in rootless
		res.IsRootlessAllowed = hostSubUIDRange.Length > 0
		return res, nil
	}

	parts := strings.SplitN(raw, ":", 2)
	uidVal, errU := strconv.Atoi(parts[0])
	if errU == nil {
		res.UID = uidVal
		res.IsNumeric = true
		if len(parts) == 2 {
			gidVal, errG := strconv.Atoi(parts[1])
			if errG == nil {
				res.GID = gidVal
			}
		}
	} else {
		// Non-numeric user (e.g. "nobody", "www-data")
		res.IsNumeric = false
		res.UID = 65534 // fallback nobody
		res.GID = 65534
	}

	if res.UID == 0 {
		res.IsRoot = true
	}

	// Check if UID fits in host SubID length
	if uint32(res.UID) < hostSubUIDRange.Length {
		res.IsRootlessAllowed = true
	} else if hostSubUIDRange.Length > 0 && res.UID == 0 {
		res.IsRootlessAllowed = true
	}

	return res, nil
}

// FormatUserNamespaceMapping returns a human-readable summary of user namespace compatibility.
func FormatUserNamespaceMapping(configJSON []byte, hostRange SubIDRange) string {
	mapping, err := ValidateUserNamespaceMapping(configJSON, hostRange)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("User Namespace Mapping Evaluation:\n"))
	sb.WriteString(fmt.Sprintf("  Config User: %q\n", mapping.RawUser))
	sb.WriteString(fmt.Sprintf("  Parsed UID:GID: %d:%d (numeric: %t)\n", mapping.UID, mapping.GID, mapping.IsNumeric))
	sb.WriteString(fmt.Sprintf("  Runs as Root: %t\n", mapping.IsRoot))
	sb.WriteString(fmt.Sprintf("  Rootless Mode Compatible: %t", mapping.IsRootlessAllowed))
	return sb.String()
}
