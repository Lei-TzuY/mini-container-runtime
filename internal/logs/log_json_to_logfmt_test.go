package logs

import (
	"strings"
	"testing"
)

func TestJSONToLogfmt_ValidJSON(t *testing.T) {
	input := `{"level":"error","msg":"connection refused","ts":"2026-08-21T12:00:00Z","port":8080}`
	got := JSONToLogfmt(input)

	for _, kv := range []string{"level=error", "msg=\"connection refused\"", "port=8080"} {
		if !strings.Contains(got, kv) {
			t.Errorf("expected %q in %q", kv, got)
		}
	}
}

func TestJSONToLogfmt_NonJSON(t *testing.T) {
	input := "plain text log line"
	got := JSONToLogfmt(input)
	if got != input {
		t.Errorf("non-JSON line should be unchanged, got %q", got)
	}
}

func TestJSONToLogfmt_EmptyObject(t *testing.T) {
	got := JSONToLogfmt(`{}`)
	if got != "" {
		t.Errorf("expected empty string for empty JSON object, got %q", got)
	}
}

func TestConvertJSONStreamToLogfmt(t *testing.T) {
	lines := []string{
		`{"level":"info","msg":"started"}`,
		"non-json line",
		`{"level":"warn","code":404}`,
	}

	out := ConvertJSONStreamToLogfmt(lines)
	if len(out) != 3 {
		t.Fatalf("expected 3 output lines, got %d", len(out))
	}
	if !strings.Contains(out[0], "level=info") {
		t.Errorf("line 0 missing level=info: %q", out[0])
	}
	if out[1] != "non-json line" {
		t.Errorf("line 1 should be unchanged: %q", out[1])
	}
	if !strings.Contains(out[2], "code=404") {
		t.Errorf("line 2 missing code=404: %q", out[2])
	}
}
