//go:build linux

package container

import (
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

func TestOpenExecTargetsCapturesCgroupNamespace(t *testing.T) {
	startTime, err := ProcessStartTime(os.Getpid())
	if err != nil {
		t.Fatalf("current process start time: %v", err)
	}
	targets, err := openExecTargets(os.Getpid(), startTime)
	if err != nil {
		t.Fatalf("open current process exec targets: %v", err)
	}
	defer targets.close()

	var cgroupFD int = -1
	for _, target := range targets.ns {
		if target.name == "cgroup" {
			if target.flag != unix.CLONE_NEWCGROUP {
				t.Fatalf("cgroup namespace flag=%#x, want %#x", target.flag, unix.CLONE_NEWCGROUP)
			}
			cgroupFD = target.fd
			break
		}
	}
	if cgroupFD < 0 {
		t.Fatal("cgroup namespace target was not captured")
	}

	var captured unix.Stat_t
	if err := unix.Fstat(cgroupFD, &captured); err != nil {
		t.Fatalf("fstat captured cgroup namespace: %v", err)
	}
	var current unix.Stat_t
	if err := unix.Stat("/proc/self/ns/cgroup", &current); err != nil {
		t.Fatalf("stat current cgroup namespace: %v", err)
	}
	if captured.Ino != current.Ino || captured.Dev != current.Dev {
		t.Fatalf("captured cgroup namespace=(dev=%d ino=%d), current=(dev=%d ino=%d)", captured.Dev, captured.Ino, current.Dev, current.Ino)
	}
}
