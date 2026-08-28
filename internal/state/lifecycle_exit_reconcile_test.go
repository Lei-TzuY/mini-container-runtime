package state

import (
	"os"
	"testing"
	"time"
)

func newLifecycleTestStore(t *testing.T, id string) *Store {
	t.Helper()
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Save(&Container{ID: id, Status: StatusCreated, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	return st
}

func TestUnknownExitCodeCanBeUpgradedForSameExitedIdentity(t *testing.T) {
	const (
		id    = "ctr-exit-upgrade"
		pid   = 4242
		start = 987654
	)
	st := newLifecycleTestStore(t, id)
	if err := st.MarkRunning(id, pid, start, time.Now()); err != nil {
		t.Fatal(err)
	}

	fallbackFinished := time.Now()
	changed, err := st.MarkStoppedIfIdentity(id, pid, start, -1, fallbackFinished)
	if err != nil || !changed {
		t.Fatalf("unknown stop: changed=%v err=%v", changed, err)
	}
	if _, err := os.Stat(exitedIdentityPath(st.ctrDir, id)); err != nil {
		t.Fatalf("expected exited-identity tombstone: %v", err)
	}

	authoritativeFinished := fallbackFinished.Add(time.Second)
	changed, err = st.MarkStoppedIfIdentity(id, pid, start, 23, authoritativeFinished)
	if err != nil || !changed {
		t.Fatalf("authoritative upgrade: changed=%v err=%v", changed, err)
	}

	got, err := st.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusStopped || got.ExitCode != 23 {
		t.Fatalf("unexpected reconciled state: %+v", got)
	}
	if got.FinishedAt == nil || !got.FinishedAt.Equal(authoritativeFinished) {
		t.Fatalf("finished_at=%v, want %v", got.FinishedAt, authoritativeFinished)
	}
	gotPID, gotStart, ok, err := st.GetExitedIdentity(id)
	if err != nil || !ok || gotPID != pid || gotStart != start {
		t.Fatalf("stopped generation identity lost after reconciliation: pid=%d start=%d ok=%v err=%v", gotPID, gotStart, ok, err)
	}

	changed, err = st.MarkStoppedIfIdentity(id, pid, start, -1, time.Now())
	if err != nil || changed {
		t.Fatalf("late unknown writer changed authoritative state: changed=%v err=%v", changed, err)
	}
	got, err = st.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.ExitCode != 23 {
		t.Fatalf("authoritative exit code was downgraded: %d", got.ExitCode)
	}
}

func TestKnownExitPersistsStoppedGenerationIdentity(t *testing.T) {
	const (
		id    = "ctr-known-exit"
		pid   = 5151
		start = 6161
	)
	st := newLifecycleTestStore(t, id)
	if err := st.MarkRunning(id, pid, start, time.Now()); err != nil {
		t.Fatal(err)
	}
	if changed, err := st.MarkStoppedIfIdentity(id, pid, start, 0, time.Now()); err != nil || !changed {
		t.Fatalf("known stop: changed=%v err=%v", changed, err)
	}
	gotPID, gotStart, ok, err := st.GetExitedIdentity(id)
	if err != nil || !ok || gotPID != pid || gotStart != start {
		t.Fatalf("durable stopped generation identity: pid=%d start=%d ok=%v err=%v", gotPID, gotStart, ok, err)
	}
}

func TestUnknownExitCodeRejectsDifferentExitedIdentity(t *testing.T) {
	const id = "ctr-exit-stale"
	st := newLifecycleTestStore(t, id)
	if err := st.MarkRunning(id, 100, 200, time.Now()); err != nil {
		t.Fatal(err)
	}
	if changed, err := st.MarkStoppedIfIdentity(id, 100, 200, -1, time.Now()); err != nil || !changed {
		t.Fatalf("fallback stop: changed=%v err=%v", changed, err)
	}

	for _, tc := range []struct {
		pid   int
		start uint64
	}{
		{pid: 100, start: 201},
		{pid: 101, start: 200},
	} {
		changed, err := st.MarkStoppedIfIdentity(id, tc.pid, tc.start, 7, time.Now())
		if err != nil {
			t.Fatalf("stale upgrade %d/%d: %v", tc.pid, tc.start, err)
		}
		if changed {
			t.Fatalf("stale identity %d/%d upgraded exit code", tc.pid, tc.start)
		}
	}

	got, err := st.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.ExitCode != -1 {
		t.Fatalf("stale identity changed exit code: %d", got.ExitCode)
	}
}

func TestRestartClearsExitedIdentityTombstone(t *testing.T) {
	const id = "ctr-exit-restart"
	st := newLifecycleTestStore(t, id)
	if err := st.MarkRunning(id, 100, 200, time.Now()); err != nil {
		t.Fatal(err)
	}
	if changed, err := st.MarkStoppedIfIdentity(id, 100, 200, 0, time.Now()); err != nil || !changed {
		t.Fatalf("stop: changed=%v err=%v", changed, err)
	}
	if err := st.MarkRunning(id, 300, 400, time.Now()); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if _, err := os.Stat(exitedIdentityPath(st.ctrDir, id)); !os.IsNotExist(err) {
		t.Fatalf("old exited identity survived restart: %v", err)
	}

	changed, err := st.MarkStoppedIfIdentity(id, 100, 200, 9, time.Now())
	if err != nil || changed {
		t.Fatalf("old lifecycle changed restarted state: changed=%v err=%v", changed, err)
	}
	got, err := st.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusRunning || got.PID != 300 || got.PIDStartTime != 400 {
		t.Fatalf("restart identity was corrupted: %+v", got)
	}
}

func TestDeleteRemovesExitedIdentityTombstone(t *testing.T) {
	const id = "ctr-exit-delete"
	st := newLifecycleTestStore(t, id)
	if err := st.MarkRunning(id, 100, 200, time.Now()); err != nil {
		t.Fatal(err)
	}
	if changed, err := st.MarkStoppedIfIdentity(id, 100, 200, 0, time.Now()); err != nil || !changed {
		t.Fatalf("stop: changed=%v err=%v", changed, err)
	}
	path := exitedIdentityPath(st.ctrDir, id)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected tombstone before delete: %v", err)
	}
	if err := st.Delete(id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("tombstone survived container delete: %v", err)
	}
}
