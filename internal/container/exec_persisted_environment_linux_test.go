//go:build linux

package container

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
)

const execEnvironmentPayloadHelperKey = "MINICONTAINER_TEST_EXEC_ENV_PAYLOAD"

func TestExecEnvironmentOverridesHostForRealPayload(t *testing.T) {
	t.Setenv("MINICONTAINER_EXEC_ENV_SOURCE", "host")
	env := mergeEnvironment(os.Environ(), []string{
		"MINICONTAINER_EXEC_ENV_SOURCE=container",
		"MINICONTAINER_EXEC_ENV_ONLY=present",
	})
	env = append(env, execEnvironmentPayloadHelperKey+"=1", execSentinelEnv, execStartTimeKey+"=123", execWorkDirKey+"=/")

	var stdout, stderr bytes.Buffer
	if err := runExecPayload([]string{os.Args[0], "-test.run=^TestExecEnvironmentPayloadHelper$"}, payloadEnvironment(env), nil, &stdout, &stderr); err != nil {
		t.Fatalf("runExecPayload: %v stderr=%q", err, stderr.String())
	}
	got := strings.TrimSpace(stdout.String())
	if got != "source=container only=present runtime=false" {
		t.Fatalf("payload environment=%q", got)
	}
}

func TestExecEnvironmentPayloadHelper(t *testing.T) {
	if os.Getenv(execEnvironmentPayloadHelperKey) != "1" {
		return
	}
	_, runtimeSentinel := os.LookupEnv(execSentinelKey)
	fmt.Printf("source=%s only=%s runtime=%t\n", os.Getenv("MINICONTAINER_EXEC_ENV_SOURCE"), os.Getenv("MINICONTAINER_EXEC_ENV_ONLY"), runtimeSentinel)
}

func TestMergeEnvironmentUsesLastConfiguredValue(t *testing.T) {
	got := mergeEnvironment([]string{"A=host", "B=keep"}, []string{"A=container", "C=new"})
	joined := strings.Join(got, "\n")
	for _, want := range []string{"A=container", "B=keep", "C=new"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("merged environment %q missing %q", joined, want)
		}
	}
	if strings.Contains(joined, "A=host") {
		t.Fatalf("host value survived override: %q", joined)
	}
}
