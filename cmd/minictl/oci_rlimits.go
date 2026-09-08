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
	processOOMScoreAdjEnv  = "MINICONTAINER_PROCESS_OOM_SCORE_ADJ"
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

// UnmarshalJSON extends the strict OCI bundle decoder with bounded process
// policy surfaces the runtime can execute today. Policies are encoded as
// reserved parent/runtime markers in Process.Env: existing managed-run state
// already persists that environment across restart, while container init
// consumes and removes the markers before launching the payload.
func (c *ociBundleConfig) UnmarshalJSON(data []byte) error {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		return err
	}

	var rlimitsRaw json.RawMessage
	var oomScoreAdjRaw json.RawMessage
	if processRaw, ok := document["process"]; ok {
		var process map[string]json.RawMessage
		if err := json.Unmarshal(processRaw, &process); err != nil {
			return err
		}
		rlimitsRaw = process["rlimits"]
		oomScoreAdjRaw = process["oomScoreAdj"]
		delete(process, "rlimits")
		delete(process, "oomScoreAdj")
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

	if len(oomScoreAdjRaw) != 0 {
		if runtime.GOOS != "linux" {
			return fmt.Errorf("OCI process.oomScoreAdj requires linux")
		}
		if bytes.Equal(bytes.TrimSpace(oomScoreAdjRaw), []byte("null")) {
			return fmt.Errorf("OCI process.oomScoreAdj must be an integer")
		}
		var oomScoreAdj int
		if err := json.Unmarshal(oomScoreAdjRaw, &oomScoreAdj); err != nil {
			return fmt.Errorf("decode process.oomScoreAdj: %w", err)
		}
		if oomScoreAdj < -1000 || oomScoreAdj > 1000 {
			return fmt.Errorf("OCI process.oomScoreAdj %d is outside [-1000,1000]", oomScoreAdj)
		}
		for _, entry := range c.Process.Env {
			key := entry
			if j := strings.IndexByte(key, '='); j >= 0 {
				key = key[:j]
			}
			if key == processOOMScoreAdjEnv {
				return fmt.Errorf("process.env key %q conflicts with internal oom score policy", key)
			}
		}
		c.Process.Env = append(c.Process.Env, fmt.Sprintf("%s=%d", processOOMScoreAdjEnv, oomScoreAdj))
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
