//go:build linux

package container

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

const processSchedulerEnv = "MINICONTAINER_PROCESS_SCHEDULER"

var processSchedulerThreadID int

// OCI process.scheduler is a per-thread Linux property. Pin the re-executed
// container-init or exec generation to the thread on which the policy is
// installed so the eventual payload exec inherits the requested scheduler
// policy instead of migrating to an unconfigured Go runtime thread.
func init() {
	if os.Getenv(sentinelEnvKey) != "1" && os.Getenv(execSentinelKey) != "1" {
		return
	}
	raw, ok := os.LookupEnv(processSchedulerEnv)
	if !ok {
		return
	}
	if err := os.Unsetenv(processSchedulerEnv); err != nil {
		failProcessSchedulerInit("clear runtime marker: %v", err)
	}

	policyName, niceText, ok := strings.Cut(strings.TrimSpace(raw), ":")
	if !ok || policyName == "" || niceText == "" || strings.Contains(niceText, ":") {
		failProcessSchedulerInit("invalid runtime marker %q", raw)
	}
	nice, err := strconv.Atoi(niceText)
	if err != nil || nice < -20 || nice > 19 {
		failProcessSchedulerInit("invalid nice value %q", niceText)
	}
	policy, ok := processSchedulerPolicy(policyName)
	if !ok {
		failProcessSchedulerInit("invalid policy %q", policyName)
	}

	runtime.LockOSThread()
	attr := &unix.SchedAttr{
		Size:   unix.SizeofSchedAttr,
		Policy: policy,
		Nice:   int32(nice),
	}
	if err := unix.SchedSetAttr(0, attr, 0); err != nil {
		failProcessSchedulerInit("set %s nice=%d: %v", policyName, nice, err)
	}
	processSchedulerThreadID = unix.Gettid()
}

func processSchedulerPolicy(name string) (uint32, bool) {
	switch name {
	case "SCHED_OTHER":
		return unix.SCHED_NORMAL, true
	case "SCHED_BATCH":
		return unix.SCHED_BATCH, true
	case "SCHED_IDLE":
		return unix.SCHED_IDLE, true
	default:
		return 0, false
	}
}

func failProcessSchedulerInit(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "container init: process scheduler: "+format+"\n", args...)
	os.Exit(126)
}
