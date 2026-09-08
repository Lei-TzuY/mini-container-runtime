//go:build linux

package container

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const processOOMScoreAdjEnv = "MINICONTAINER_PROCESS_OOM_SCORE_ADJ"

// OCI process.oomScoreAdj belongs to the container process. Apply it in the
// re-executed container-init generation so the PID 1 supervisor and payload
// inherit the kernel policy, then remove the internal marker before exec.
func init() {
	if os.Getenv(sentinelEnvKey) != "1" {
		return
	}
	raw, ok := os.LookupEnv(processOOMScoreAdjEnv)
	if !ok {
		return
	}
	if err := os.Unsetenv(processOOMScoreAdjEnv); err != nil {
		failProcessOOMScoreInit("clear runtime marker: %v", err)
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		failProcessOOMScoreInit("invalid runtime marker %q: %v", raw, err)
	}
	if value < -1000 || value > 1000 {
		failProcessOOMScoreInit("value %d is outside [-1000,1000]", value)
	}
	if err := os.WriteFile("/proc/self/oom_score_adj", []byte(strconv.Itoa(value)), 0); err != nil {
		failProcessOOMScoreInit("set oom_score_adj=%d: %v", value, err)
	}
}

func failProcessOOMScoreInit(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "container init: process oom score: "+format+"\n", args...)
	os.Exit(126)
}
