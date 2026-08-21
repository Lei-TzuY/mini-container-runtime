package logs

import (
	"strings"
	"testing"
	"time"
)

func TestLogTimeWindowGrouper_GroupLines(t *testing.T) {
	lines := []string{
		"2026-08-21T12:00:05Z [INFO] task started",
		"2026-08-21T12:00:15Z [WARN] high memory",
		"2026-08-21T12:00:45Z [ERROR] failed to connect",
		"2026-08-21T12:01:05Z [INFO] retry successful",
		"2026-08-21T12:01:20Z [INFO] job completed",
	}

	grouper := NewLogTimeWindowGrouper(time.Minute)
	buckets := grouper.GroupLines(lines, time.Now())

	if len(buckets) != 2 {
		t.Fatalf("expected 2 minute buckets, got %d", len(buckets))
	}

	// Bucket 0 (12:00:00)
	if buckets[0].TotalLines != 3 {
		t.Errorf("bucket 0 TotalLines = %d, want 3", buckets[0].TotalLines)
	}
	if buckets[0].ErrorCount != 1 {
		t.Errorf("bucket 0 ErrorCount = %d, want 1", buckets[0].ErrorCount)
	}
	if buckets[0].WarnCount != 1 {
		t.Errorf("bucket 0 WarnCount = %d, want 1", buckets[0].WarnCount)
	}

	// Bucket 1 (12:01:00)
	if buckets[1].TotalLines != 2 {
		t.Errorf("bucket 1 TotalLines = %d, want 2", buckets[1].TotalLines)
	}
	if buckets[1].ErrorCount != 0 {
		t.Errorf("bucket 1 ErrorCount = %d, want 0", buckets[1].ErrorCount)
	}
}

func TestFormatWindowBuckets(t *testing.T) {
	buckets := []LogWindowBucket{
		{
			StartTime:  time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC),
			EndTime:    time.Date(2026, 8, 21, 12, 1, 0, 0, time.UTC),
			TotalLines: 5,
			ErrorCount: 1,
			WarnCount:  0,
		},
	}

	out := FormatWindowBuckets(buckets)
	if !strings.Contains(out, "Log Window Distribution (1 windows):") {
		t.Errorf("expected header in %q", out)
	}
	if !strings.Contains(out, "5 lines") {
		t.Errorf("expected '5 lines' in %q", out)
	}
}
