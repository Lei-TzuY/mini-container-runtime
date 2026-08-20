package logs

import (
	"strings"
	"testing"
	"time"
)

func TestLogAlertEngine_DefaultTriggers(t *testing.T) {
	engine := NewDefaultAlertEngine()
	now := time.Now()

	lines := []string{
		"2026-08-20 [INFO] Worker started",
		"2026-08-20 [FATAL] panic: runtime error: nil pointer dereference",
		"2026-08-20 [ERROR] Out of Memory: Kill process 1234 (app)",
	}

	events := engine.ScanStream(lines, now)
	if len(events) != 2 {
		t.Fatalf("expected 2 alert events, got %d", len(events))
	}

	if events[0].TriggerName != "Panic Detected" {
		t.Errorf("events[0].TriggerName = %q, want 'Panic Detected'", events[0].TriggerName)
	}
	if events[1].TriggerName != "OOM Killer" {
		t.Errorf("events[1].TriggerName = %q, want 'OOM Killer'", events[1].TriggerName)
	}

	summary := FormatAlertSummary(events)
	if !strings.Contains(summary, "Alerts Detected: 2 events") {
		t.Errorf("expected 2 events in summary, got %q", summary)
	}
}

func TestLogAlertEngine_CustomRule(t *testing.T) {
	engine := &LogAlertEngine{}
	if err := engine.AddRule("Disk Full", `(?i)no space left on device`, "HIGH"); err != nil {
		t.Fatal(err)
	}

	events := engine.ScanLine("write /data/db: no space left on device", time.Now())
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Severity != "HIGH" {
		t.Errorf("Severity = %q, want HIGH", events[0].Severity)
	}
}
