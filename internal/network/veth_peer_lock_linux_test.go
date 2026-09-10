//go:build linux

package network

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

const vethPeerLockHelperEnv = "MINICONTAINER_VETH_PEER_LOCK_HELPER"

func TestVethPeerHandoffLockSerializesProcesses(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	firstReadyR, firstReadyW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer firstReadyR.Close()
	firstReleaseR, firstReleaseW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer firstReleaseW.Close()

	first := exec.Command(os.Args[0], "-test.run=^TestVethPeerHandoffLockHelper$")
	first.Env = append(os.Environ(), vethPeerLockHelperEnv+"=hold")
	first.ExtraFiles = []*os.File{firstReadyW, firstReleaseR}
	first.Stdout = os.Stdout
	first.Stderr = os.Stderr
	if err := first.Start(); err != nil {
		t.Fatalf("start first lock holder: %v", err)
	}
	defer func() { _ = first.Process.Kill() }()
	_ = firstReadyW.Close()
	_ = firstReleaseR.Close()
	readLockSignal(t, firstReadyR, "first acquired")

	secondReadyR, secondReadyW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer secondReadyR.Close()
	secondStartedR, secondStartedW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer secondStartedR.Close()

	second := exec.Command(os.Args[0], "-test.run=^TestVethPeerHandoffLockHelper$")
	second.Env = append(os.Environ(), vethPeerLockHelperEnv+"=once")
	second.ExtraFiles = []*os.File{secondReadyW, secondStartedW}
	second.Stdout = os.Stdout
	second.Stderr = os.Stderr
	if err := second.Start(); err != nil {
		t.Fatalf("start second lock contender: %v", err)
	}
	defer func() { _ = second.Process.Kill() }()
	_ = secondReadyW.Close()
	_ = secondStartedW.Close()
	readLockSignal(t, secondStartedR, "second started")

	if err := unix.SetNonblock(int(secondReadyR.Fd()), true); err != nil {
		t.Fatalf("set second ready pipe nonblocking: %v", err)
	}
	var b [1]byte
	_, err = unix.Read(int(secondReadyR.Fd()), b[:])
	if err == nil {
		t.Fatal("second process acquired veth peer handoff lock while first process still held it")
	}
	if !errors.Is(err, syscall.EAGAIN) && !errors.Is(err, syscall.EWOULDBLOCK) {
		t.Fatalf("probe second acquisition while first holds lock: %v", err)
	}
	if err := unix.SetNonblock(int(secondReadyR.Fd()), false); err != nil {
		t.Fatalf("restore blocking second ready pipe: %v", err)
	}

	if _, err := firstReleaseW.Write([]byte{1}); err != nil {
		t.Fatalf("release first holder: %v", err)
	}
	readLockSignal(t, secondReadyR, "second acquired after release")

	if err := first.Wait(); err != nil {
		t.Fatalf("first holder failed: %v", err)
	}
	if err := second.Wait(); err != nil {
		t.Fatalf("second contender failed: %v", err)
	}
}

func TestVethPeerHandoffLockHelper(t *testing.T) {
	mode := os.Getenv(vethPeerLockHelperEnv)
	if mode == "" {
		return
	}
	acquired := os.NewFile(3, "acquired")
	control := os.NewFile(4, "control")
	if acquired == nil || control == nil {
		t.Fatal("missing helper pipe")
	}
	defer acquired.Close()
	defer control.Close()

	if mode == "once" {
		if _, err := control.Write([]byte{1}); err != nil {
			t.Fatalf("signal contender started: %v", err)
		}
	}

	if err := withVethPeerHandoffLock(func() error {
		if _, err := acquired.Write([]byte{1}); err != nil {
			return err
		}
		if mode == "hold" {
			var release [1]byte
			_, err := io.ReadFull(control, release[:])
			return err
		}
		return nil
	}); err != nil {
		t.Fatalf("veth peer handoff lock: %v", err)
	}
}

func readLockSignal(t *testing.T, r *os.File, label string) {
	t.Helper()
	var b [1]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		t.Fatalf("read %s signal: %v", label, err)
	}
}
