// Package imagestore provides OCI image configuration inspection utilities.
// This file implements a StopSignal auditor that resolves OCI Image Config StopSignal
// definitions into canonical POSIX signal names and numeric codes.

package imagestore

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// StopSignalSummary represents evaluated stop signal and graceful timeout parameters.
type StopSignalSummary struct {
	DeclaredSignal    string
	CanonicalSignal   string
	SignalNumber      int
	IsGraceful        bool // false only for signals that cannot be handled gracefully, such as SIGKILL
	DefaultTimeoutSec int
}

var knownSignals = map[string]int{
	"SIGHUP":   1,
	"SIGINT":   2,
	"SIGQUIT":  3,
	"SIGKILL":  9,
	"SIGUSR1":  10,
	"SIGUSR2":  12,
	"SIGTERM":  15,
	"SIGWINCH": 28,
}

const maxLinuxSignalNumber = 64

// EvaluateStopSignal parses image config StopSignal and returns structured signal data.
func EvaluateStopSignal(configJSON []byte) (StopSignalSummary, error) {
	var cfg struct {
		Config struct {
			StopSignal string `json:"StopSignal,omitempty"`
		} `json:"config"`
	}
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		return StopSignalSummary{}, fmt.Errorf("parse image config for stop signal: %w", err)
	}

	raw := strings.TrimSpace(cfg.Config.StopSignal)
	summary := StopSignalSummary{
		DeclaredSignal:    raw,
		CanonicalSignal:   "SIGTERM",
		SignalNumber:      15,
		IsGraceful:        true,
		DefaultTimeoutSec: 10,
	}

	if raw == "" {
		return summary, nil
	}

	upper := strings.ToUpper(raw)
	if num, err := strconv.Atoi(upper); err == nil {
		if num <= 0 || num > maxLinuxSignalNumber {
			return StopSignalSummary{}, fmt.Errorf("invalid stop signal number %d: expected 1-%d", num, maxLinuxSignalNumber)
		}

		summary.SignalNumber = num
		summary.CanonicalSignal = fmt.Sprintf("SIG_%d", num)
		for name, code := range knownSignals {
			if code == num {
				summary.CanonicalSignal = name
				break
			}
		}
		if num == knownSignals["SIGKILL"] {
			summary.IsGraceful = false
			summary.DefaultTimeoutSec = 0
		}
		return summary, nil
	}

	if !strings.HasPrefix(upper, "SIG") {
		upper = "SIG" + upper
	}

	code, ok := knownSignals[upper]
	if !ok {
		return StopSignalSummary{}, fmt.Errorf("unknown stop signal %q", raw)
	}

	summary.CanonicalSignal = upper
	summary.SignalNumber = code
	if code == knownSignals["SIGKILL"] {
		summary.IsGraceful = false
		summary.DefaultTimeoutSec = 0
	}

	return summary, nil
}

// FormatStopSignal returns a human-readable stop signal summary.
func FormatStopSignal(configJSON []byte) string {
	summary, err := EvaluateStopSignal(configJSON)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}

	return fmt.Sprintf("Stop Signal: %s (num: %d, graceful: %t, timeout: %ds)",
		summary.CanonicalSignal, summary.SignalNumber, summary.IsGraceful, summary.DefaultTimeoutSec)
}
