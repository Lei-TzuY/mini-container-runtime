package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
)

const (
	processRlimitNOFILEEnv  = "MINICONTAINER_PROCESS_RLIMIT_NOFILE"
	processRlimitCOREEnv    = "MINICONTAINER_PROCESS_RLIMIT_CORE"
	processRlimitFSIZEEnv   = "MINICONTAINER_PROCESS_RLIMIT_FSIZE"
	processRlimitSTACKEnv   = "MINICONTAINER_PROCESS_RLIMIT_STACK"
	processRlimitNPROCEnv   = "MINICONTAINER_PROCESS_RLIMIT_NPROC"
	processRlimitMEMLOCKEnv = "MINICONTAINER_PROCESS_RLIMIT_MEMLOCK"
	processRlimitASEnv      = "MINICONTAINER_PROCESS_RLIMIT_AS"
	processRlimitDATAEnv    = "MINICONTAINER_PROCESS_RLIMIT_DATA"
	processOOMScoreAdjEnv   = "MINICONTAINER_PROCESS_OOM_SCORE_ADJ"
	processIOPriorityEnv    = "MINICONTAINER_PROCESS_IO_PRIORITY"
	processSchedulerEnv     = "MINICONTAINER_PROCESS_SCHEDULER"
	processCPUSetCPUsEnv    = "MINICONTAINER_CGROUP_CPUSET_CPUS"
	processCPUSetMemsEnv    = "MINICONTAINER_CGROUP_CPUSET_MEMS"
)

var processRlimitRuntimeEnv = map[string]string{
	"RLIMIT_NOFILE":  processRlimitNOFILEEnv,
	"RLIMIT_CORE":    processRlimitCOREEnv,
	"RLIMIT_FSIZE":   processRlimitFSIZEEnv,
	"RLIMIT_STACK":   processRlimitSTACKEnv,
	"RLIMIT_NPROC":   processRlimitNPROCEnv,
	"RLIMIT_MEMLOCK": processRlimitMEMLOCKEnv,
	"RLIMIT_AS":      processRlimitASEnv,
	"RLIMIT_DATA":    processRlimitDATAEnv,
}

type ociProcessRlimitConfig struct {
	Type string `json:"type"`
	Hard uint64 `json:"hard"`
	Soft uint64 `json:"soft"`
}

type ociProcessIOPriorityConfig struct {
	Class    string `json:"class"`
	Priority *int   `json:"priority"`
}

type ociProcessSchedulerConfig struct {
	Policy   string   `json:"policy"`
	Nice     *int32   `json:"nice"`
	Priority *int32   `json:"priority"`
	Flags    []string `json:"flags"`
	Runtime  *uint64  `json:"runtime"`
	Deadline *uint64  `json:"deadline"`
	Period   *uint64  `json:"period"`
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
	var ioPriorityRaw json.RawMessage
	var schedulerRaw json.RawMessage
	var cpusetCPUsRaw json.RawMessage
	var cpusetMemsRaw json.RawMessage
	var memoryReservationRaw json.RawMessage
	if processRaw, ok := document["process"]; ok {
		var process map[string]json.RawMessage
		if err := json.Unmarshal(processRaw, &process); err != nil {
			return err
		}
		rlimitsRaw = process["rlimits"]
		oomScoreAdjRaw = process["oomScoreAdj"]
		ioPriorityRaw = process["ioPriority"]
		schedulerRaw = process["scheduler"]
		delete(process, "rlimits")
		delete(process, "oomScoreAdj")
		delete(process, "ioPriority")
		delete(process, "scheduler")
		cleanProcess, err := json.Marshal(process)
		if err != nil {
			return err
		}
		document["process"] = cleanProcess
	}
	if linuxRaw, ok := document["linux"]; ok && !bytes.Equal(bytes.TrimSpace(linuxRaw), []byte("null")) {
		var linux map[string]json.RawMessage
		if err := json.Unmarshal(linuxRaw, &linux); err != nil {
			return err
		}
		if resourcesRaw, ok := linux["resources"]; ok && !bytes.Equal(bytes.TrimSpace(resourcesRaw), []byte("null")) {
			var resources map[string]json.RawMessage
			if err := json.Unmarshal(resourcesRaw, &resources); err != nil {
				return err
			}
			if memoryRaw, ok := resources["memory"]; ok && !bytes.Equal(bytes.TrimSpace(memoryRaw), []byte("null")) {
				var memory map[string]json.RawMessage
				if err := json.Unmarshal(memoryRaw, &memory); err != nil {
					return err
				}
				memoryReservationRaw = memory["reservation"]
				delete(memory, "reservation")
				cleanMemory, err := json.Marshal(memory)
				if err != nil {
					return err
				}
				resources["memory"] = cleanMemory
			}
			if cpuRaw, ok := resources["cpu"]; ok && !bytes.Equal(bytes.TrimSpace(cpuRaw), []byte("null")) {
				var cpu map[string]json.RawMessage
				if err := json.Unmarshal(cpuRaw, &cpu); err != nil {
					return err
				}
				cpusetCPUsRaw = cpu["cpus"]
				cpusetMemsRaw = cpu["mems"]
				delete(cpu, "cpus")
				delete(cpu, "mems")
				cleanCPU, err := json.Marshal(cpu)
				if err != nil {
					return err
				}
				resources["cpu"] = cleanCPU
			}
			cleanResources, err := json.Marshal(resources)
			if err != nil {
				return err
			}
			linux["resources"] = cleanResources
		}
		cleanLinux, err := json.Marshal(linux)
		if err != nil {
			return err
		}
		document["linux"] = cleanLinux
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

	if len(memoryReservationRaw) != 0 {
		if runtime.GOOS != "linux" {
			return fmt.Errorf("OCI linux.resources.memory.reservation requires linux")
		}
		if bytes.Equal(bytes.TrimSpace(memoryReservationRaw), []byte("null")) {
			return fmt.Errorf("OCI linux.resources.memory.reservation must be greater than zero")
		}
		var reservation int64
		if err := json.Unmarshal(memoryReservationRaw, &reservation); err != nil {
			return fmt.Errorf("decode linux.resources.memory.reservation: %w", err)
		}
		if reservation <= 0 {
			return fmt.Errorf("linux.resources.memory.reservation must be greater than zero")
		}
		for _, entry := range c.Process.Env {
			key := entry
			if j := strings.IndexByte(key, '='); j >= 0 {
				key = key[:j]
			}
			if key == processMemoryHighEnv {
				return fmt.Errorf("process.env key %q conflicts with internal memory reservation policy", key)
			}
		}
		c.Process.Env = append(c.Process.Env, fmt.Sprintf("%s=%d", processMemoryHighEnv, reservation))
	}

	appendCPUSetMarker := func(raw json.RawMessage, field, marker string) error {
		if len(raw) == 0 {
			return nil
		}
		if runtime.GOOS != "linux" {
			return fmt.Errorf("OCI linux.resources.cpu.%s requires linux", field)
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("OCI linux.resources.cpu.%s must be a non-empty string", field)
		}
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return fmt.Errorf("decode linux.resources.cpu.%s: %w", field, err)
		}
		value = strings.TrimSpace(value)
		if value == "" || strings.ContainsAny(value, "\x00\r\n") {
			return fmt.Errorf("OCI linux.resources.cpu.%s must be a non-empty single-line cpuset expression", field)
		}
		for _, entry := range c.Process.Env {
			key := entry
			if j := strings.IndexByte(key, '='); j >= 0 {
				key = key[:j]
			}
			if key == marker {
				return fmt.Errorf("process.env key %q conflicts with internal cpuset policy", key)
			}
		}
		c.Process.Env = append(c.Process.Env, marker+"="+value)
		return nil
	}
	if err := appendCPUSetMarker(cpusetCPUsRaw, "cpus", processCPUSetCPUsEnv); err != nil {
		return err
	}
	if err := appendCPUSetMarker(cpusetMemsRaw, "mems", processCPUSetMemsEnv); err != nil {
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

	if len(ioPriorityRaw) != 0 {
		if runtime.GOOS != "linux" {
			return fmt.Errorf("OCI process.ioPriority requires linux")
		}
		if bytes.Equal(bytes.TrimSpace(ioPriorityRaw), []byte("null")) {
			return fmt.Errorf("OCI process.ioPriority must be an object")
		}
		var ioPriority ociProcessIOPriorityConfig
		dec = json.NewDecoder(bytes.NewReader(ioPriorityRaw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&ioPriority); err != nil {
			return fmt.Errorf("decode process.ioPriority: %w", err)
		}
		switch ioPriority.Class {
		case "IOPRIO_CLASS_RT", "IOPRIO_CLASS_BE", "IOPRIO_CLASS_IDLE":
		default:
			return fmt.Errorf("OCI process.ioPriority class %q is not supported", ioPriority.Class)
		}
		if ioPriority.Priority == nil {
			return fmt.Errorf("OCI process.ioPriority.priority is required")
		}
		if *ioPriority.Priority < 0 || *ioPriority.Priority > 7 {
			return fmt.Errorf("OCI process.ioPriority priority %d is outside [0,7]", *ioPriority.Priority)
		}
		for _, entry := range c.Process.Env {
			key := entry
			if j := strings.IndexByte(key, '='); j >= 0 {
				key = key[:j]
			}
			if key == processIOPriorityEnv {
				return fmt.Errorf("process.env key %q conflicts with internal io priority policy", key)
			}
		}
		c.Process.Env = append(c.Process.Env, fmt.Sprintf("%s=%s:%d", processIOPriorityEnv, ioPriority.Class, *ioPriority.Priority))
	}

	if len(schedulerRaw) != 0 {
		if runtime.GOOS != "linux" {
			return fmt.Errorf("OCI process.scheduler requires linux")
		}
		if bytes.Equal(bytes.TrimSpace(schedulerRaw), []byte("null")) {
			return fmt.Errorf("OCI process.scheduler must be an object")
		}
		var scheduler ociProcessSchedulerConfig
		dec = json.NewDecoder(bytes.NewReader(schedulerRaw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&scheduler); err != nil {
			return fmt.Errorf("decode process.scheduler: %w", err)
		}
		switch scheduler.Policy {
		case "SCHED_OTHER", "SCHED_BATCH", "SCHED_IDLE":
		default:
			return fmt.Errorf("OCI process.scheduler policy %q is not supported", scheduler.Policy)
		}
		nice := int32(0)
		if scheduler.Nice != nil {
			nice = *scheduler.Nice
		}
		if nice < -20 || nice > 19 {
			return fmt.Errorf("OCI process.scheduler nice %d is outside [-20,19]", nice)
		}
		if scheduler.Priority != nil && *scheduler.Priority != 0 {
			return fmt.Errorf("OCI process.scheduler priority must be 0 for %s", scheduler.Policy)
		}
		if len(scheduler.Flags) != 0 {
			return fmt.Errorf("OCI process.scheduler flags are not supported for %s", scheduler.Policy)
		}
		if scheduler.Runtime != nil && *scheduler.Runtime != 0 {
			return fmt.Errorf("OCI process.scheduler runtime must be 0 for %s", scheduler.Policy)
		}
		if scheduler.Deadline != nil && *scheduler.Deadline != 0 {
			return fmt.Errorf("OCI process.scheduler deadline must be 0 for %s", scheduler.Policy)
		}
		if scheduler.Period != nil && *scheduler.Period != 0 {
			return fmt.Errorf("OCI process.scheduler period must be 0 for %s", scheduler.Policy)
		}
		for _, entry := range c.Process.Env {
			key := entry
			if j := strings.IndexByte(key, '='); j >= 0 {
				key = key[:j]
			}
			if key == processSchedulerEnv {
				return fmt.Errorf("process.env key %q conflicts with internal scheduler policy", key)
			}
		}
		c.Process.Env = append(c.Process.Env, fmt.Sprintf("%s=%s:%d", processSchedulerEnv, scheduler.Policy, nice))
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
