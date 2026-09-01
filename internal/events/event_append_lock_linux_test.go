//go:build linux

package events

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEventLogAppendSerializesIndependentOpeners(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.log")
	first, err := openEventLogForAppend(path)
	if err != nil {
		t.Fatalf("open first writer: %v", err)
	}

	type result struct {
		file *lockedEventLogFile
		err  error
	}
	secondResult := make(chan result, 1)
	go func() {
		file, openErr := openEventLogForAppend(path)
		secondResult <- result{file: file, err: openErr}
	}()

	select {
	case got := <-secondResult:
		if got.file != nil {
			_ = got.file.Close()
		}
		_ = first.Close()
		t.Fatalf("second writer bypassed held lock: %v", got.err)
	case <-time.After(75 * time.Millisecond):
		// Expected: the independently opened sidecar blocks on LOCK_EX.
	}

	if err := first.Close(); err != nil {
		t.Fatalf("close first writer: %v", err)
	}
	select {
	case got := <-secondResult:
		if got.err != nil {
			t.Fatalf("open second writer after unlock: %v", got.err)
		}
		if got.file == nil {
			t.Fatal("second writer returned nil file")
		}
		if err := got.file.Close(); err != nil {
			t.Fatalf("close second writer: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second writer remained blocked after first writer closed")
	}
}

func TestEventLogAppendRejectsSymlinkWriterLock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.log")
	victim := filepath.Join(dir, "victim")
	const original = "do-not-touch"
	if err := os.WriteFile(victim, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, path+".lock"); err != nil {
		t.Fatal(err)
	}

	file, err := openEventLogForAppend(path)
	if file != nil {
		_ = file.Close()
	}
	if err == nil {
		t.Fatal("expected symlink writer lock to be rejected")
	}
	if !strings.Contains(err.Error(), "writer lock") {
		t.Fatalf("unexpected error: %v", err)
	}
	contents, readErr := os.ReadFile(victim)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(contents) != original {
		t.Fatalf("symlink target was modified: %q", contents)
	}
	if _, statErr := os.Lstat(path); !os.IsNotExist(statErr) {
		t.Fatalf("events.log unexpectedly created, stat err=%v", statErr)
	}
}

func TestEventLogAppendRejectsHardLinkedWriterLock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.log")
	lockPath := path + ".lock"
	if err := os.WriteFile(lockPath, []byte("lock"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(lockPath, filepath.Join(dir, "lock-alias")); err != nil {
		t.Fatal(err)
	}

	file, err := openEventLogForAppend(path)
	if file != nil {
		_ = file.Close()
	}
	if err == nil {
		t.Fatal("expected hard-linked writer lock to be rejected")
	}
	if !strings.Contains(err.Error(), "unexpected hard links") {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, statErr := os.Lstat(path); !os.IsNotExist(statErr) {
		t.Fatalf("events.log unexpectedly created, stat err=%v", statErr)
	}
}
