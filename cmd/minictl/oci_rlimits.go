package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
)

const (
	processRlimitNOFILEEnv = "MINICONTAINER_PROCESS_RLIMIT_NOFILE"
	processRlimitCOREEnv   = "MINICONTAINER_PROCESS_RLIMIT_CORE"
	processRlimitFSIZEEnv  = "MINICONTAINER_PROCESS_RLIMIT_FSIZE"
	processRlimitSTACKEnv  = "MINICONTAINER_PROCESS_RLIMIT_STACK"
	processRlimitNPROCEnv  = "MINICONTAINER_PROCESS_RLIMIT_NPROC"
)

var processRlimitRuntimeEnv = map[string]string{
	"RLIMIT_NOFILE": processRlimitNOFILEEnv,
	"RLIMIT_CORE":   processRlimitCOREEnv,
	"RLIMIT_FSIZE":  processRlimitFSIZEEnv,
	"RLIMIT_STACK":  processRlimitSTACKEnv,
	"RLIMIT_NPROC":  processRlimitNPROCEnv,
}

type ociProcessRlimitConfig struct {
	Type string `json:"type"`
	Hard uint64 `json:"hard"`
	Soft uint64 `json:"soft"`
}

// UnmarshalJSON extends the strict OCI bundle decoder with the bounded rlimit
// surface the runtime can execute today. Each rlimit policy is encoded as a
// reserved parent/runtime marker in Process.Env: existing managed-run state
// already persists that environment across restart, while container init
// consumes and removes the markers before launching the payload.
func (c *ociBundleConfig) UnmarshalJSON(data []byte) error {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		return err
	}

	var rlimitsRaw json.RawMessage
	if processRaw, ok := document["process"]; ok {
		var process map[string]json.RawMessage
		if err := json.Unmarshal(processRaw, &process); err != nil {
			return err
		}
		rlimitsRaw = process["rlimits"]
		delete(process, "rlimits")
		cleanProcess, err := json.Marshal(process)
		if err != nil {
			return err
		}
		document["process"] = cleanProcess
	}

	cleanDocument, err := json.Marshal(document)
	if err != nil {
		return err
	}
	type plainOCIBundleConfig ociBundleConfig
	dec := json.NewDecoder(bytes.NewReader(cleanDocument))
	dec.DisallowUnknownFields()
	if err := dec.Decode((*plainOCIBundleConfig)(c)); err != nil {
		return err
	}
	if len(rlimitsRaw) == 0 {
		return nil
	}
	if runtime.GOOS != "linux" {
		return fmt.Errorf("OCI process.rlimits requires linux")
	}

	var rlimits []ociProcessRlimitConfig
	dec = json.NewDecoder(bytes.NewReader(rlimitsRaw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&rlimits); err != nil {
		return fmt.Errorf("decode process.rlimits: %w", err)
	}
	if len(rlimits) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(rlimits))
	for i := range rlimits {
		limit := &rlimits[i]
		marker, ok := processRlimitRuntimeEnv[limit.Type]
		if !ok {
			return fmt.Errorf("OCI process.rlimits type %q is not supported", limit.Type)
		}
		if _, ok := seen[limit.Type]; ok {
			return fmt.Errorf("duplicate OCI process.rlimits type %q", limit.Type)
		}
		if limit.Soft > limit.Hard {
			return fmt.Errorf("OCI process.rlimits %s soft limit %d exceeds hard limit %d", limit.Type, limit.Soft, limit.Hard)
		}
		seen[limit.Type] = struct{}{}

		for _, entry := range c.Process.Env {
			key := entry
			if j := strings.IndexByte(key, '='); j >= 0 {
				key = key[:j]
			}
			if key == marker {
				return fmt.Errorf("process.env key %q conflicts with internal rlimit policy", key)
			}
		}
		c.Process.Env = append(c.Process.Env, fmt.Sprintf("%s=%d:%d", marker, limit.Soft, limit.Hard))
	}
	return nil
}
