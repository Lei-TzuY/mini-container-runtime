package events

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenEventLogForStreamFollowWaitsForInitialLogCreation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	logPath := LogPath()
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		t.Fatal(err)
	}

	type result struct {
		file *os.File
		err  error
	}
	resultCh := make(chan result, 1)
	go func() {
		f, err := openEventLogForStream(logPath, true)
		resultCh <- result{file: f, err: err}
	}()

	select {
	case got := <-resultCh:
		if got.file != nil {
			got.file.Close()
		}
		t.Fatalf("follow returned before the first event log existed: %v", got.err)
	case <-time.After(75 * time.Millisecond):
	}

	if err := os.WriteFile(logPath, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}

	select {
	case got := <-resultCh:
		if got.err != nil {
			t.Fatalf("follow did not open newly created event log: %v", got.err)
		}
		if got.file == nil {
			t.Fatal("follow returned nil event log without error")
		}
		got.file.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("follow did not observe initial event log creation")
	}
}

func TestOpenEventLogForStreamNonFollowPreservesMissingLogBehavior(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	logPath := LogPath()
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		t.Fatal(err)
	}

	f, err := openEventLogForStream(logPath, false)
	if f != nil {
		f.Close()
		t.Fatal("non-follow unexpectedly opened missing event log")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("non-follow error = %v, want os.ErrNotExist", err)
	}
}

func TestOpenEventLogForStreamFollowFailsClosedOnSymlinkLog(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	logPath := LogPath()
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "events-target.log")
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, logPath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	resultCh := make(chan error, 1)
	go func() {
		f, err := openEventLogForStream(logPath, true)
		if f != nil {
			f.Close()
		}
		resultCh <- err
	}()

	select {
	case err := <-resultCh:
		if err == nil {
			t.Fatal("follow accepted symlink event log")
		}
		if errors.Is(err, os.ErrNotExist) {
			t.Fatalf("symlink rejection was misclassified as retryable absence: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("follow retried a non-ENOENT storage safety failure")
	}
}
