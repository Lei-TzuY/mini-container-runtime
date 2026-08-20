package imagestore

import (
	"strings"
	"testing"
)

func TestResolveImageEnvironment(t *testing.T) {
	configJSON := `{
		"config": {
			"Env": [
				"BASE_DIR=/opt/app",
				"LOG_DIR=${BASE_DIR}/logs",
				"DEBUG=${UNSET_VAR:-true}",
				"DATA_PATH=$LOG_DIR/data"
			]
		}
	}`

	envs, err := ResolveImageEnvironment([]byte(configJSON))
	if err != nil {
		t.Fatalf("ResolveImageEnvironment failed: %v", err)
	}

	expected := map[string]string{
		"BASE_DIR":  "/opt/app",
		"LOG_DIR":   "/opt/app/logs",
		"DEBUG":     "true",
		"DATA_PATH": "/opt/app/logs/data",
	}

	for _, e := range envs {
		parts := strings.SplitN(e, "=", 2)
		if wantVal, exists := expected[parts[0]]; exists {
			if parts[1] != wantVal {
				t.Errorf("env %s = %q, want %q", parts[0], parts[1], wantVal)
			}
		}
	}
}

func TestFormatResolvedEnvironment(t *testing.T) {
	configJSON := `{"config":{"Env":["FOO=BAR"]}}`
	got := FormatResolvedEnvironment([]byte(configJSON))
	if !strings.Contains(got, "1 variables (fully resolved)") {
		t.Errorf("expected resolved summary in %q", got)
	}
}
