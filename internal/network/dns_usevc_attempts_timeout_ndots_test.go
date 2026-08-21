package network

import (
	"strings"
	"testing"
)

func TestGenerateDNSUseVCAttemptsTimeoutNdotsConfig(t *testing.T) {
	tests := []struct {
		name     string
		attempts int
		timeout  int
		ndots    int
		want     string
	}{
		{
			name:     "custom values",
			attempts: 3,
			timeout:  7,
			ndots:    2,
			want:     "options use-vc attempts:3 timeout:7 ndots:2\n",
		},
		{
			name:     "defaults on zero",
			attempts: 0,
			timeout:  0,
			ndots:    0,
			want:     "options use-vc attempts:2 timeout:5 ndots:1\n",
		},
		{
			name:     "negative values",
			attempts: -1,
			timeout:  -1,
			ndots:    -1,
			want:     "options use-vc attempts:2 timeout:5 ndots:1\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := GenerateDNSUseVCAttemptsTimeoutNdotsConfig(tc.attempts, tc.timeout, tc.ndots)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGenerateDNSUseVCAttemptsTimeoutNdotsConfig_Contents(t *testing.T) {
	got := GenerateDNSUseVCAttemptsTimeoutNdotsConfig(4, 15, 3)
	for _, kw := range []string{"use-vc", "attempts:4", "timeout:15", "ndots:3"} {
		if !strings.Contains(got, kw) {
			t.Errorf("expected %q in %q", kw, got)
		}
	}
}
