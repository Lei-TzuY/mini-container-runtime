//go:build linux && !amd64 && !arm64

package container

// blockedSyscalls is empty for architectures other than amd64/arm64.
// The seccomp filter will be installed but allow all syscalls, acting as
// a no-op.  Extend this file (or add a new arch-specific file) to add
// coverage for additional architectures.
var blockedSyscalls = []uint32{}
