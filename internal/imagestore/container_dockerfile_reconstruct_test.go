package imagestore

import (
	"strings"
	"testing"
)

func TestReconstructDockerfile(t *testing.T) {
	configJSON := `{
		"history": [
			{"created_by": "/bin/sh -c #(nop)  ENV PATH=/usr/local/bin:/usr/bin", "empty_layer": true},
			{"created_by": "/bin/sh -c apt-get update && apt-get install -y curl"},
			{"created_by": "/bin/sh -c #(nop) WORKDIR /app", "empty_layer": true},
			{"created_by": "/bin/sh -c #(nop)  EXPOSE 8080", "empty_layer": true},
			{"created_by": "/bin/sh -c #(nop)  CMD [\"python\" \"app.py\"]", "empty_layer": true}
		]
	}`

	instructions, err := ReconstructDockerfile([]byte(configJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(instructions) != 5 {
		t.Fatalf("expected 5 instructions, got %d", len(instructions))
	}

	if instructions[0].Command != "ENV" {
		t.Errorf("inst[0].Command = %q, want ENV", instructions[0].Command)
	}
	if instructions[1].Command != "RUN" {
		t.Errorf("inst[1].Command = %q, want RUN", instructions[1].Command)
	}
	if !strings.Contains(instructions[1].Arguments, "apt-get") {
		t.Errorf("inst[1].Arguments missing apt-get: %q", instructions[1].Arguments)
	}
	if instructions[2].Command != "WORKDIR" {
		t.Errorf("inst[2].Command = %q, want WORKDIR", instructions[2].Command)
	}
	if instructions[4].Command != "CMD" {
		t.Errorf("inst[4].Command = %q, want CMD", instructions[4].Command)
	}
}

func TestFormatReconstructedDockerfile(t *testing.T) {
	configJSON := `{"history":[{"created_by":"/bin/sh -c echo hello"}]}`
	got := FormatReconstructedDockerfile([]byte(configJSON))
	if !strings.Contains(got, "RUN echo hello") {
		t.Errorf("expected 'RUN echo hello' in %q", got)
	}
}

func TestReconstructDockerfile_EmptyHistory(t *testing.T) {
	got := FormatReconstructedDockerfile([]byte(`{"history":[]}`))
	if !strings.Contains(got, "no history") {
		t.Errorf("expected 'no history' in %q", got)
	}
}
