package state

import (
	"reflect"
	"testing"
	"time"
)

func TestRestartSpecRoundTrip(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()

	rec := &Container{ID: "restartable", Status: StatusStopped, RootFS: "/rootfs", Command: []string{"/bin/app"}, CreatedAt: time.Now()}
	if err := st.Save(rec); err != nil {
		t.Fatalf("Save: %v", err)
	}
	want := RestartSpec{RootFS: "/rootfs", Command: []string{"/bin/app", "serve"}, Env: []string{"A=B"}, WorkDir: "/work", Hostname: "ctr"}
	if err := st.SaveRestartSpec(rec.ID, want); err != nil {
		t.Fatalf("SaveRestartSpec: %v", err)
	}
	got, err := st.RestartSpec(rec.ID)
	if err != nil {
		t.Fatalf("RestartSpec: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RestartSpec=%#v, want %#v", got, want)
	}
	got.Command[0] = "mutated"
	got.Env[0] = "mutated"
	again, err := st.RestartSpec(rec.ID)
	if err != nil {
		t.Fatalf("RestartSpec again: %v", err)
	}
	if !reflect.DeepEqual(again, want) {
		t.Fatalf("durable spec mutated through caller slice: %#v", again)
	}
}

func TestSaveRestartSpecRequiresExistingOwner(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()
	if err := st.SaveRestartSpec("missing", RestartSpec{RootFS: "/rootfs", Command: []string{"/bin/app"}}); err == nil {
		t.Fatal("expected missing owner error")
	}
}

func TestRestartSpecRejectsIncompleteSpec(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()
	rec := &Container{ID: "incomplete", Status: StatusStopped, RootFS: "/rootfs", Command: []string{"/bin/app"}, CreatedAt: time.Now()}
	if err := st.Save(rec); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := st.SaveRestartSpec(rec.ID, RestartSpec{RootFS: "/rootfs"}); err == nil {
		t.Fatal("expected empty command rejection")
	}
}
