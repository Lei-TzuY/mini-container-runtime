package state

import (
	"errors"
	"os"
	"testing"
	"time"
)

func TestExitedIdentityRequirementFailureDoesNotPublishIdentity(t *testing.T) {
	const (
		id    = "ctr-marker-failure"
		pid   = 9101
		start = 9201
	)
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Save(&Container{ID: id, Status: StatusCreated, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkRunning(id, pid, start, time.Now()); err != nil {
		t.Fatal(err)
	}

	// Force the capability-marker atomic rename to fail. The stronger ordering
	// invariant is that no new .exit generation key may appear until the marker
	// proving modern semantics is durable.
	if err := os.Mkdir(exitedIdentityRequiredPath(st.ctrDir, id), 0o700); err != nil {
		t.Fatal(err)
	}
	changed, err := st.MarkStoppedIfIdentity(id, pid, start, 0, time.Now())
	if err == nil || changed {
		t.Fatalf("MarkStoppedIfIdentity changed=%v err=%v, want failed transition", changed, err)
	}
	if _, statErr := os.Stat(exitedIdentityPath(st.ctrDir, id)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("identity published before requirement marker: stat err=%v", statErr)
	}
	c, getErr := st.Get(id)
	if getErr != nil {
		t.Fatal(getErr)
	}
	if c.Status != StatusRunning || c.PID != pid || c.PIDStartTime != start {
		t.Fatalf("failed stop changed running lifecycle: status=%s pid=%d start=%d", c.Status, c.PID, c.PIDStartTime)
	}
}

func TestExitedIdentityWriteFailureLeavesFailClosedCapabilityAndCanRetry(t *testing.T) {
	const (
		id    = "ctr-identity-failure"
		pid   = 9301
		start = 9401
	)
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Save(&Container{ID: id, Status: StatusCreated, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkRunning(id, pid, start, time.Now()); err != nil {
		t.Fatal(err)
	}

	// Force .exit publication to fail after the marker is durable. Leaving the
	// capability marker is intentional: it is container-scoped and makes every
	// future stopped record fail closed if its exact identity is unavailable.
	identityPath := exitedIdentityPath(st.ctrDir, id)
	if err := os.Mkdir(identityPath, 0o700); err != nil {
		t.Fatal(err)
	}
	changed, err := st.MarkStoppedIfIdentity(id, pid, start, 0, time.Now())
	if err == nil || changed {
		t.Fatalf("MarkStoppedIfIdentity changed=%v err=%v, want failed transition", changed, err)
	}
	required, reqErr := st.exitedIdentityRequiredUnlocked(id)
	if reqErr != nil {
		t.Fatal(reqErr)
	}
	if !required {
		t.Fatal("identity write failure lost durable fail-closed capability marker")
	}
	c, getErr := st.Get(id)
	if getErr != nil {
		t.Fatal(getErr)
	}
	if c.Status != StatusRunning || c.PID != pid || c.PIDStartTime != start {
		t.Fatalf("failed stop changed running lifecycle: status=%s pid=%d start=%d", c.Status, c.PID, c.PIDStartTime)
	}

	if err := os.Remove(identityPath); err != nil {
		t.Fatal(err)
	}
	changed, err = st.MarkStoppedIfIdentity(id, pid, start, 0, time.Now())
	if err != nil || !changed {
		t.Fatalf("retry stop changed=%v err=%v", changed, err)
	}
	stopped, err := st.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	gotPID, gotStart, current, ok, err := st.GetExitedIdentityForStoppedRevision(id, stopped.Revision)
	if err != nil || !current || !ok || gotPID != pid || gotStart != start {
		t.Fatalf("retried stop lost exact identity: pid=%d start=%d current=%v ok=%v err=%v", gotPID, gotStart, current, ok, err)
	}
}
