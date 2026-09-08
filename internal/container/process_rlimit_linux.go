//go:build linux

package container

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

const (
	processRlimitNOFILEEnv = "MINICONTAINER_PROCESS_RLIMIT_NOFILE"
	processRlimitCOREEnv   = "MINICONTAINER_PROCESS_RLIMIT_CORE"
	processRlimitFSIZEEnv  = "MINICONTAINER_PROCESS_RLIMIT_FSIZE"
	processRlimitSTACKEnv  = "MINICONTAINER_PROCESS_RLIMIT_STACK"
)

type processRlimitRuntimePolicy struct {
	marker   string
	resource int
	name     string
}

var processRlimitRuntimePolicies = []processRlimitRuntimePolicy{
	{marker: processRlimitNOFILEEnv, resource: unix.RLIMIT_NOFILE, name: "RLIMIT_NOFILE"},
	{marker: processRlimitCOREEnv, resource: unix.RLIMIT_CORE, name: "RLIMIT_CORE"},
	{marker: processRlimitFSIZEEnv, resource: unix.RLIMIT_FSIZE, name: "RLIMIT_FSIZE"},
	{marker: processRlimitSTACKEnv, resource: unix.RLIMIT_STACK, name: "RLIMIT_STACK"},
}

// applyProcessRlimitRuntimeMarker runs in the re-executed container-init
// generation. OCI process rlimits belong to that container process; the
// internal PID 1 supervisor and its payload therefore inherit the same kernel
// limits. Markers are removed before the payload environment is constructed.
func init() {
	if os.Getenv(sentinelEnvKey) != "1" {
		return
	}
	for _, policy := range processRlimitRuntimePolicies {
		applyProcessRlimitRuntimeMarker(policy)
	}
}

func applyProcessRlimitRuntimeMarker(policy processRlimitRuntimePolicy) {
	raw, ok := os.LookupEnv(policy.marker)
	if !ok {
		return
	}
	if err := os.Unsetenv(policy.marker); err != nil {
		failProcessRlimitInit("clear %s runtime marker: %v", policy.name, err)
	}
	parts := strings.Split(raw, ":")
	if len(parts) != 2 {
		failProcessRlimitInit("invalid %s runtime marker %q", policy.name, raw)
	}
	soft, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		failProcessRlimitInit("invalid %s soft limit in runtime marker %q: %v", policy.name, raw, err)
	}
	hard, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		failProcessRlimitInit("invalid %s hard limit in runtime marker %q: %v", policy.name, raw, err)
	}
	if soft > hard {
		failProcessRlimitInit("%s soft limit %d exceeds hard limit %d", policy.name, soft, hard)
	}
	if err := unix.Setrlimit(policy.resource, &unix.Rlimit{Cur: soft, Max: hard}); err != nil {
		failProcessRlimitInit("set %s %d:%d: %v", policy.name, soft, hard, err)
	}
}

func failProcessRlimitInit(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "container init: process rlimit: "+format+"\n", args...)
	os.Exit(126)
}
