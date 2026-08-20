// Package logs provides container log processing utilities.
// This file implements an alert engine that monitors container logs
// for critical keywords (panic, OOM, segfault) and fires structured alert events.

package logs

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// AlertEvent represents a fired alert trigger.
type AlertEvent struct {
	Timestamp   time.Time
	TriggerName string
	MatchedLine string
	Severity    string
}

// AlertRule defines matching criteria for firing an alert.
type AlertRule struct {
	Name     string
	Pattern  *regexp.Regexp
	Severity string
}

// LogAlertEngine monitors log lines and generates alert events.
type LogAlertEngine struct {
	Rules []AlertRule
}

// NewDefaultAlertEngine creates a LogAlertEngine with default rules for critical errors.
func NewDefaultAlertEngine() *LogAlertEngine {
	engine := &LogAlertEngine{}
	engine.AddRule("Panic Detected", `(?i)\b(panic|goroutine \d+ \[running\])\b`, "CRITICAL")
	engine.AddRule("OOM Killer", `(?i)\b(oom-killer|out of memory|killed process|oom_kill)\b`, "CRITICAL")
	engine.AddRule("Segmentation Fault", `(?i)\b(segmentation fault|sigsegv|core dumped)\b`, "FATAL")
	engine.AddRule("Deadlock / Timeout", `(?i)\b(deadlock detected|context deadline exceeded)\b`, "ERROR")
	return engine
}

// AddRule compiles and adds a new alert rule.
func (ae *LogAlertEngine) AddRule(name, pattern, severity string) error {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("compile alert rule %q: %w", name, err)
	}
	ae.Rules = append(ae.Rules, AlertRule{
		Name:     name,
		Pattern:  re,
		Severity: severity,
	})
	return nil
}

// ScanLine checks a single log line against all rules and returns any triggered alerts.
func (ae *LogAlertEngine) ScanLine(line string, now time.Time) []AlertEvent {
	var events []AlertEvent
	for _, rule := range ae.Rules {
		if rule.Pattern.MatchString(line) {
			events = append(events, AlertEvent{
				Timestamp:   now,
				TriggerName: rule.Name,
				MatchedLine: line,
				Severity:    rule.Severity,
			})
		}
	}
	return events
}

// ScanStream processes a slice of log lines, returning all detected alert events.
func (ae *LogAlertEngine) ScanStream(lines []string, baseTime time.Time) []AlertEvent {
	var allEvents []AlertEvent
	for _, line := range lines {
		events := ae.ScanLine(line, baseTime)
		allEvents = append(allEvents, events...)
	}
	return allEvents
}

// FormatAlertSummary formats detected alert events into an alert summary string.
func FormatAlertSummary(events []AlertEvent) string {
	if len(events) == 0 {
		return "Alerts: (none detected)"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Alerts Detected: %d events\n", len(events)))
	for i, e := range events {
		sb.WriteString(fmt.Sprintf("  [%d] [%s] %s: %s\n",
			i+1, e.Severity, e.TriggerName, strings.TrimSpace(e.MatchedLine)))
	}
	return strings.TrimRight(sb.String(), "\n")
}
