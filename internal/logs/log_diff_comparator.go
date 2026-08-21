// Package logs provides container log processing utilities.
// This file implements a run-to-run container log diff comparator
// to diagnose nondeterministic crashes and diverging execution traces between container runs.

package logs

import (
	"fmt"
	"strings"
)

// LogStreamDiff holds structural difference metrics between two log runs.
type LogStreamDiff struct {
	TotalLinesA     int
	TotalLinesB     int
	CommonLines     int
	OnlyInA         []string
	OnlyInB         []string
	SimilarityRatio float64 // 0.0 (completely distinct) to 1.0 (identical)
}

// CompareLogStreams computes line set differences and execution similarity between two log runs.
func CompareLogStreams(linesA, linesB []string) LogStreamDiff {
	setA := make(map[string]int)
	for _, l := range linesA {
		cleaned := strings.TrimSpace(l)
		if cleaned != "" {
			setA[cleaned]++
		}
	}

	setB := make(map[string]int)
	for _, l := range linesB {
		cleaned := strings.TrimSpace(l)
		if cleaned != "" {
			setB[cleaned]++
		}
	}

	diff := LogStreamDiff{
		TotalLinesA: len(linesA),
		TotalLinesB: len(linesB),
	}

	for line := range setA {
		if _, exists := setB[line]; exists {
			diff.CommonLines++
		} else {
			diff.OnlyInA = append(diff.OnlyInA, line)
		}
	}

	for line := range setB {
		if _, exists := setA[line]; !exists {
			diff.OnlyInB = append(diff.OnlyInB, line)
		}
	}

	unionSize := len(setA) + len(diff.OnlyInB)
	if unionSize > 0 {
		diff.SimilarityRatio = float64(diff.CommonLines) / float64(unionSize)
	} else {
		diff.SimilarityRatio = 1.0
	}

	return diff
}

// FormatLogStreamDiff returns a formatted comparison summary.
func FormatLogStreamDiff(diff LogStreamDiff) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Log Stream Comparison (Similarity: %.1f%%):\n", diff.SimilarityRatio*100.0))
	sb.WriteString(fmt.Sprintf("  Run A: %d lines | Run B: %d lines | Common: %d lines\n",
		diff.TotalLinesA, diff.TotalLinesB, diff.CommonLines))
	sb.WriteString(fmt.Sprintf("  Divergent lines: %d only in A, %d only in B",
		len(diff.OnlyInA), len(diff.OnlyInB)))
	return sb.String()
}
