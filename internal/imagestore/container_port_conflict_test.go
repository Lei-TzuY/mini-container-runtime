package imagestore

import (
	"strings"
	"testing"
)

func TestDetectPortConflicts(t *testing.T) {
	configs := map[string][]byte{
		"nginx":   []byte(`{"config":{"ExposedPorts":{"80/tcp":{},"443/tcp":{}}}}`),
		"apache":  []byte(`{"config":{"ExposedPorts":{"80/tcp":{},"8080/tcp":{}}}}`),
		"traefik": []byte(`{"config":{"ExposedPorts":{"443/tcp":{},"8080/tcp":{}}}}`),
	}

	conflicts, err := DetectPortConflicts(configs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conflicts) != 3 {
		t.Fatalf("expected 3 conflicts (80, 443, 8080), got %d", len(conflicts))
	}
}

func TestDetectPortConflicts_NoConflict(t *testing.T) {
	configs := map[string][]byte{
		"app1": []byte(`{"config":{"ExposedPorts":{"3000/tcp":{}}}}`),
		"app2": []byte(`{"config":{"ExposedPorts":{"4000/tcp":{}}}}`),
	}

	conflicts, err := DetectPortConflicts(configs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts, got %d", len(conflicts))
	}
}

func TestFormatPortConflicts(t *testing.T) {
	conflicts := []PortConflict{
		{Port: "80/tcp", Images: []string{"nginx", "apache"}},
	}
	got := FormatPortConflicts(conflicts)
	if !strings.Contains(got, "1 overlapping") {
		t.Errorf("expected '1 overlapping' in %q", got)
	}
}
