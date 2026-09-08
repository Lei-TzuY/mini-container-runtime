package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
)

const processRlimitNOFILEEnv = "MINICONTAINER_PROCESS_RLIMIT_NOFILE"

type ociProcessRlimitConfig struct {
	Type string `json:"type"`
	Hard uint64 `json:"hard"`
	Soft uint64 `json:"soft"`
}

// UnmarshalJSON extends the strict OCI bundle decoder with the bounded rlimit
// surface the runtime can execute today. The rlimit policy is encoded as a
// reserved parent/runtime marker in Process.Env: existing managed-run state
// already persists that environment across restart, while container init
// consumes and removes the marker before launching the payload.
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

	var nofile *ociProcessRlimitConfig
	for i := range rlimits {
		limit := &rlimits[i]
		if limit.Type != "RLIMIT_NOFILE" {
			return fmt.Errorf("OCI process.rlimits type %q is not supported", limit.Type)
		}
		if nofile != nil {
			return fmt.Errorf("duplicate OCI process.rlimits type %q", limit.Type)
		}
		if limit.Soft > limit.Hard {
			return fmt.Errorf("OCI process.rlimits %s soft limit %d exceeds hard limit %d", limit.Type, limit.Soft, limit.Hard)
		}
		nofile = limit
	}

	for _, entry := range c.Process.Env {
		key := entry
		if i := strings.IndexByte(key, '='); i >= 0 {
			key = key[:i]
		}
		if key == processRlimitNOFILEEnv {
			return fmt.Errorf("process.env key %q conflicts with internal rlimit policy", key)
		}
	}
	c.Process.Env = append(c.Process.Env, fmt.Sprintf("%s=%d:%d", processRlimitNOFILEEnv, nofile.Soft, nofile.Hard))
	return nil
}
