//go:build linux

package network

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

type bridgeCommandResult struct {
	out []byte
	err error
}

func scriptedBridgeRunner(t *testing.T, results []bridgeCommandResult, calls *[][]string) bridgeCommandRunner {
	t.Helper()
	return func(args ...string) ([]byte, error) {
		*calls = append(*calls, append([]string(nil), args...))
		idx := len(*calls) - 1
		if idx >= len(results) {
			t.Fatalf("unexpected bridge command %v", args)
		}
		return results[idx].out, results[idx].err
	}
}

func TestCreateBridgeJoinsAddressSetupAndRollbackFailures(t *testing.T) {
	addrErr := errors.New("address setup failed")
	rollbackErr := errors.New("bridge delete failed")
	var calls [][]string
	run := scriptedBridgeRunner(t, []bridgeCommandResult{
		{},
		{out: []byte("addr output"), err: addrErr},
		{out: []byte("delete output"), err: rollbackErr},
	}, &calls)

	err := createBridgeWith("demo", "172.28.0.1/24", false, run)
	if !errors.Is(err, addrErr) || !errors.Is(err, rollbackErr) {
		t.Fatalf("error=%v, want both setup and rollback causes", err)
	}
	if !strings.Contains(err.Error(), "addr output") || !strings.Contains(err.Error(), "delete output") {
		t.Fatalf("error=%q, want both command outputs", err)
	}
	want := [][]string{
		{"link", "add", "br-demo", "type", "bridge"},
		{"addr", "add", "172.28.0.1/24", "dev", "br-demo"},
		{"link", "delete", "br-demo"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls=%v, want %v", calls, want)
	}
}

func TestCreateBridgeJoinsLinkUpAndRollbackFailures(t *testing.T) {
	upErr := errors.New("link up failed")
	rollbackErr := errors.New("bridge delete failed")
	var calls [][]string
	run := scriptedBridgeRunner(t, []bridgeCommandResult{
		{},
		{},
		{out: []byte("up output"), err: upErr},
		{out: []byte("delete output"), err: rollbackErr},
	}, &calls)

	err := createBridgeWith("demo", "", false, run)
	if !errors.Is(err, upErr) || !errors.Is(err, rollbackErr) {
		t.Fatalf("error=%v, want both setup and rollback causes", err)
	}
	want := [][]string{
		{"link", "add", "br-demo", "type", "bridge"},
		{"addr", "add", "172.28.0.1/24", "dev", "br-demo"},
		{"link", "set", "br-demo", "up"},
		{"link", "delete", "br-demo"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls=%v, want %v", calls, want)
	}
}

func TestCreateBridgePreservesSetupFailureWhenRollbackSucceeds(t *testing.T) {
	setupErr := errors.New("address setup failed")
	var calls [][]string
	run := scriptedBridgeRunner(t, []bridgeCommandResult{
		{},
		{err: setupErr},
		{},
	}, &calls)

	err := createBridgeWith("demo", "172.28.0.1/24", false, run)
	if !errors.Is(err, setupErr) {
		t.Fatalf("error=%v, want setup cause", err)
	}
	if strings.Contains(err.Error(), "rollback bridge") {
		t.Fatalf("successful rollback reported as failure: %v", err)
	}
}

func TestBridgeNameForNetworkRejectsTruncation(t *testing.T) {
	name, err := bridgeNameForNetwork("abcdefghijkl")
	if err != nil {
		t.Fatalf("12-byte network name rejected: %v", err)
	}
	if name != "br-abcdefghijkl" || len(name) != maxLinuxInterfaceNameLen {
		t.Fatalf("bridge name=%q len=%d", name, len(name))
	}

	if _, err := bridgeNameForNetwork("abcdefghijklm"); err == nil || !strings.Contains(err.Error(), "too long") {
		t.Fatalf("overlong name error=%v", err)
	}
}

func TestCreateBridgeRejectsOverlongNameBeforeHostMutation(t *testing.T) {
	calls := 0
	run := func(args ...string) ([]byte, error) {
		calls++
		return nil, nil
	}

	err := createBridgeWith("abcdefghijkl-one", "172.28.0.1/24", false, run)
	if err == nil || !strings.Contains(err.Error(), "too long") {
		t.Fatalf("error=%v, want overlong-name rejection", err)
	}
	if calls != 0 {
		t.Fatalf("host commands=%d, want none before name validation", calls)
	}
}

func TestDeleteBridgeRejectsOverlongAliasBeforeHostMutation(t *testing.T) {
	calls := 0
	run := func(args ...string) ([]byte, error) {
		calls++
		return nil, nil
	}

	// These names historically truncated to the same host interface prefix.
	for _, name := range []string{"abcdefghijkl-one", "abcdefghijkl-two"} {
		err := deleteBridgeWith(name, false, run)
		if err == nil || !strings.Contains(err.Error(), "too long") {
			t.Fatalf("name=%q error=%v, want overlong-name rejection", name, err)
		}
	}
	if calls != 0 {
		t.Fatalf("host delete commands=%d, want none for ambiguous aliases", calls)
	}
}

func TestDeleteBridgeUsesExactCanonicalName(t *testing.T) {
	var calls [][]string
	run := scriptedBridgeRunner(t, []bridgeCommandResult{{}}, &calls)
	if err := deleteBridgeWith("abcdefghijkl", false, run); err != nil {
		t.Fatalf("delete canonical bridge: %v", err)
	}
	want := [][]string{{"link", "delete", "br-abcdefghijkl"}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls=%v, want %v", calls, want)
	}
}
