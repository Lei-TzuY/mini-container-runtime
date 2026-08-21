package imagestore

import (
	"strings"
	"testing"
)

func TestEvaluateStopSignal(t *testing.T) {
	tests := []struct {
		name         string
		json         string
		wantCanonical string
		wantNum      int
		wantGraceful bool
	}{
		{
			name:          "default unset signal",
			json:          `{"config":{}}`,
			wantCanonical: "SIGTERM",
			wantNum:       15,
			wantGraceful:  true,
		},
		{
			name:          "custom SIGQUIT",
			json:          `{"config":{"StopSignal":"SIGQUIT"}}`,
			wantCanonical: "SIGQUIT",
			wantNum:       3,
			wantGraceful:  true,
		},
		{
			name:          "numeric 9 (SIGKILL)",
			json:          `{"config":{"StopSignal":"9"}}`,
			wantCanonical: "SIGKILL",
			wantNum:       9,
			wantGraceful:  false,
		},
		{
			name:          "without SIG prefix 'INT'",
			json:          `{"config":{"StopSignal":"INT"}}`,
			wantCanonical: "SIGINT",
			wantNum:       2,
			wantGraceful:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := EvaluateStopSignal([]byte(tc.json))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.CanonicalSignal != tc.wantCanonical {
				t.Errorf("CanonicalSignal = %q, want %q", res.CanonicalSignal, tc.wantCanonical)
			}
			if res.SignalNumber != tc.wantNum {
				t.Errorf("SignalNumber = %d, want %d", res.SignalNumber, tc.wantNum)
			}
			if res.IsGraceful != tc.wantGraceful {
				t.Errorf("IsGraceful = %t, want %t", res.IsGraceful, tc.wantGraceful)
			}
		})
	}
}

func TestFormatStopSignal(t *testing.T) {
	got := FormatStopSignal([]byte(`{"config":{"StopSignal":"SIGTERM"}}`))
	if !strings.Contains(got, "Stop Signal: SIGTERM (num: 15, graceful: true") {
		t.Errorf("expected SIGTERM summary in %q", got)
	}
}
