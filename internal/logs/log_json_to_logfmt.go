// Package logs provides container log processing utilities.
// This file implements a JSON-to-Logfmt transformer that converts
// structured JSON log lines into flat key=value logfmt format.

package logs

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// JSONToLogfmt converts a JSON object log line into logfmt key=value format.
// Non-JSON lines are returned unchanged.
func JSONToLogfmt(line string) string {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return line
	}

	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(trimmed), &obj); err != nil {
		return line
	}

	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		v := obj[k]
		valStr := formatLogfmtValue(v)
		parts = append(parts, fmt.Sprintf("%s=%s", k, valStr))
	}

	return strings.Join(parts, " ")
}

// formatLogfmtValue formats a value for logfmt output.
func formatLogfmtValue(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		if strings.ContainsAny(val, " \t\n\"=") || val == "" {
			return fmt.Sprintf("%q", val)
		}
		return val
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case bool:
		return fmt.Sprintf("%t", val)
	default:
		b, _ := json.Marshal(val)
		return fmt.Sprintf("%q", string(b))
	}
}

// ConvertJSONStreamToLogfmt converts a slice of JSON log lines to logfmt format.
func ConvertJSONStreamToLogfmt(lines []string) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = JSONToLogfmt(line)
	}
	return out
}
