package events

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenEventLogGenerationForFollowFallsBackToRetainedOnlyAtStartup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.log")
	retainedPath := path + ".1"
	if err := os.WriteFile(retainedPath, []byte("retained\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var opened []string
	f, expired, err := openEventLogGenerationForFollowWith(path, time.Time{}, true, func(candidate string) (*os.File, error) {
		opened = append(opened, candidate)
		return os.Open(candidate)
	}, time.Now, func(time.Duration) {
		t.Fatal("unexpected wait while retained generation exists")
	})
	if err != nil {
		t.Fatal(err)
	}
	if expired {
		t.Fatal("retained generation must be drained before follow expires")
	}
	defer f.Close()
	if len(opened) != 2 || opened[0] != path || opened[1] != retainedPath {
		t.Fatalf("open sequence=%q, want active then retained", opened)
	}

	calls := 0
	_, expired, err = openEventLogGenerationForFollowWith(path, time.Unix(1, 0).UTC(), false, func(candidate string) (*os.File, error) {
		calls++
		if candidate != path {
			t.Fatalf("post-attach reopen attempted retained path %q", candidate)
		}
		return nil, os.ErrNotExist
	}, func() time.Time { return time.Unix(1, 0).UTC() }, func(time.Duration) {
		t.Fatal("expired reopen must not wait")
	})
	if err != nil {
		t.Fatal(err)
	}
	if !expired || calls != 1 {
		t.Fatalf("expired=%v calls=%d, want true/1", expired, calls)
	}
}

func TestFollowEventLogRecoversRetainedGenerationWithoutReplay(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.log")
	retained := Event{Timestamp: time.Now().Add(-time.Second).UTC(), Type: EventStop, ContainerID: "retained-once"}
	writeFollowTestRecord(t, path+".1", retained, true)

	writes := make(chan []byte, 8)
	result := make(chan error, 1)
	go func() {
		result <- followEventLogFile(path, StreamOptions{Follow: true, JSON: true, Until: time.Now().Add(900 * time.Millisecond)}, notifyingWriter{writes: writes})
	}()

	select {
	case first := <-writes:
		if !bytes.Contains(first, []byte("retained-once")) {
			t.Fatalf("first event=%q, want retained generation", first)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for retained generation")
	}

	active := Event{Timestamp: time.Now().UTC(), Type: EventStart, ContainerID: "recovered-active"}
	writeFollowTestRecord(t, path, active, true)

	var observed bytes.Buffer
	deadline := time.After(2 * time.Second)
	for !strings.Contains(observed.String(), "recovered-active") {
		select {
		case chunk := <-writes:
			observed.Write(chunk)
		case err := <-result:
			if err != nil {
				t.Fatalf("follow interrupted rotation: %v", err)
			}
			t.Fatalf("follow exited before recovered active event: %q", observed.String())
		case <-deadline:
			t.Fatalf("timed out waiting for recovered active event: %q", observed.String())
		}
	}

	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("follow interrupted rotation: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("follow did not terminate at until deadline")
	}

	if strings.Count(observed.String(), "retained-once") != 0 {
		t.Fatalf("retained generation replayed after first delivery: %q", observed.String())
	}
}

func TestInterruptedRotationRetainedOpenFailsClosed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.log")
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path+".1"); err != nil {
		t.Fatal(err)
	}

	_, _, err := openEventLogGenerationForFollowWith(path, time.Now().Add(time.Second), true, openEventLogForRead, time.Now, func(time.Duration) {})
	if err == nil {
		t.Fatal("symlinked retained generation must fail closed")
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unsafe retained generation was treated as absent: %v", err)
	}
}
