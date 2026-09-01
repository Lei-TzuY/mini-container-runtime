package events

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeFollowTestRecord(t *testing.T, path string, evt Event, newline bool) {
	t.Helper()
	data, err := json.Marshal(evt)
	if err != nil {
		t.Fatal(err)
	}
	if newline {
		data = append(data, '\n')
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestFollowOpenEventLogRequestsReopenAfterPathReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.log")
	oldEvent := Event{Timestamp: time.Unix(1, 0).UTC(), Type: EventStart, ContainerID: "old-generation"}
	writeFollowTestRecord(t, path, oldEvent, true)

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	rotated := filepath.Join(dir, "events.log.1")
	if err := os.Rename(path, rotated); err != nil {
		t.Fatal(err)
	}
	newEvent := Event{Timestamp: time.Unix(2, 0).UTC(), Type: EventDie, ContainerID: "new-generation"}
	writeFollowTestRecord(t, path, newEvent, true)

	var out bytes.Buffer
	reopen, err := followOpenEventLog(f, path, StreamOptions{}, &out)
	if err != nil {
		t.Fatalf("follow old generation: %v", err)
	}
	if !reopen {
		t.Fatal("expected path replacement to request reopen")
	}
	got := out.String()
	if !strings.Contains(got, "old-generation") {
		t.Fatalf("durable old event was not emitted before reopen: %q", got)
	}
	if strings.Contains(got, "new-generation") {
		t.Fatalf("old descriptor unexpectedly observed replacement event: %q", got)
	}
}

func TestFollowOpenEventLogDropsTornOldTailOnReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.log")
	if err := os.WriteFile(path, []byte(`{"timestamp":"2026-01-01T00:00:00Z","type":"exec"`), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if err := os.Rename(path, filepath.Join(dir, "events.log.1")); err != nil {
		t.Fatal(err)
	}
	writeFollowTestRecord(t, path, Event{Timestamp: time.Unix(3, 0).UTC(), Type: EventExec, ContainerID: "replacement"}, true)

	var out bytes.Buffer
	reopen, err := followOpenEventLog(f, path, StreamOptions{}, &out)
	if err != nil {
		t.Fatalf("torn old tail during replacement must be ignored: %v", err)
	}
	if !reopen {
		t.Fatal("expected replacement after torn tail to request reopen")
	}
	if out.Len() != 0 {
		t.Fatalf("torn old record was emitted: %q", out.String())
	}
}

func TestWriteCompleteEventRecordStillFailsClosedOnMalformedCompleteRecord(t *testing.T) {
	var out bytes.Buffer
	err := writeCompleteEventRecord([]byte("{not-json}\n"), StreamOptions{}, &out)
	if err == nil || !strings.Contains(err.Error(), "decode event log") {
		t.Fatalf("error=%v, want complete-record corruption rejection", err)
	}
	if out.Len() != 0 {
		t.Fatalf("malformed record produced output: %q", out.String())
	}
}
