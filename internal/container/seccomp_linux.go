//go:build linux

// internal/container/seccomp_linux.go
//
// Seccomp BPF — Syscall Filtering
// ─────────────────────────────────
// Seccomp (Secure Computing Mode) restricts which syscalls a process may
// invoke.  Mode 2 (SECCOMP_MODE_FILTER) accepts a cBPF program that the
// kernel evaluates on every syscall; the program's return value decides
// the outcome:
//
//   SECCOMP_RET_ALLOW (0x7fff0000)  — permit the syscall
//   SECCOMP_RET_KILL  (0x80000000)  — kill the process immediately
//   SECCOMP_RET_ERRNO (0x00050000)  — return a synthetic errno
//   SECCOMP_RET_TRAP  (0x00030000)  — deliver SIGSYS
//
// We use a block-list approach: deny a curated set of dangerous syscalls
// (kernel module loading, kexec, ptrace, raw I/O ports, …) and allow
// everything else.  Docker's default seccomp profile blocks ~44 syscalls;
// our educational list covers the highest-risk subset.
//
// BPF program layout
// ──────────────────
//   [0]      LD W ABS 0    load syscall number (offset 0 in seccomp_data)
//   [1]      JEQ nr₀ →KILL / →next
//   [2]      RET KILL_PROCESS
//   [3]      JEQ nr₁ →KILL / →next
//   [4]      RET KILL_PROCESS
//   …        (two instructions per blocked syscall)
//   [2N+1]   RET ALLOW     default: permit
//
// PR_SET_NO_NEW_PRIVS
// ────────────────────
// Before installing the filter we set PR_SET_NO_NEW_PRIVS=1.  This
// prevents the exec'd process from gaining privileges via set-uid binaries
// and is required to install a seccomp filter without CAP_SYS_ADMIN.
// The flag is inherited across execve(2) and cannot be unset.
//
// Arch note
// ─────────
// cBPF syscall numbers are architecture-specific.  The blocked list is
// defined in seccomp_linux_amd64.go / seccomp_linux_arm64.go.

package container

import (
	"fmt"
	"syscall"
	"unsafe"
)

// ─── Constants ────────────────────────────────────────────────────────────────

const (
	prSetNoNewPrivs   = 38         // PR_SET_NO_NEW_PRIVS (not in all Go syscall pkgs)
	seccompModeFilter = 2          // SECCOMP_MODE_FILTER
	seccompRetAllow   = 0x7fff0000 // SECCOMP_RET_ALLOW
	seccompRetKill    = 0x80000000 // SECCOMP_RET_KILL_PROCESS

	// cBPF opcodes
	bpfLdWAbs  = 0x20 // BPF_LD | BPF_W | BPF_ABS  — load 32-bit word at abs offset
	bpfJmpJeqK = 0x15 // BPF_JMP | BPF_JEQ | BPF_K — jump if accumulator == constant
	bpfRetK    = 0x06 // BPF_RET | BPF_K            — return constant
)

// blockedSyscalls is defined in seccomp_linux_amd64.go / seccomp_linux_arm64.go
// / seccomp_linux_other.go — one file per architecture.

// ─── cBPF types ───────────────────────────────────────────────────────────────

// bpfInsn is Go's equivalent of struct sock_filter.
type bpfInsn struct {
	code uint16
	jt   uint8  // jump offset if condition is true  (relative to next instr)
	jf   uint8  // jump offset if condition is false
	k    uint32 // generic operand (syscall nr, return value, …)
}

// bpfProg is Go's equivalent of struct sock_fprog.
// The kernel ABI on 64-bit requires:
//
//	offset 0:  len    (uint16)
//	offset 2–7: padding (6 bytes, implicit alignment)
//	offset 8:  filter (pointer, 8 bytes)
type bpfProg struct {
	len    uint16
	_      [6]byte // alignment padding to bring filter to offset 8
	filter *bpfInsn
}

// ─── Filter construction ──────────────────────────────────────────────────────

// buildFilter returns the cBPF program that blocks blockedSyscalls.
func buildFilter() []bpfInsn {
	n := len(blockedSyscalls)
	prog := make([]bpfInsn, 0, n*2+2)

	// Load the syscall number from seccomp_data into the BPF accumulator.
	// seccomp_data.nr is at offset 0 and is 32 bits wide.
	prog = append(prog, bpfInsn{code: bpfLdWAbs, k: 0})

	// For each blocked syscall: test then kill.
	//
	// BPF_JEQ semantics: if accumulator == k  →  PC += 1 + jt
	//                    if accumulator != k  →  PC += 1 + jf
	//
	// We want:
	//   match  → fall through to the KILL instruction below (jt = 0)
	//   no match → skip the KILL instruction            (jf = 1)
	for _, nr := range blockedSyscalls {
		prog = append(prog,
			bpfInsn{code: bpfJmpJeqK, jt: 0, jf: 1, k: nr},
			bpfInsn{code: bpfRetK, k: seccompRetKill},
		)
	}

	// Default: allow everything that didn't match a blocked entry.
	prog = append(prog, bpfInsn{code: bpfRetK, k: seccompRetAllow})
	return prog
}

// ─── Installation ─────────────────────────────────────────────────────────────

// applySeccomp installs the BPF block-list filter.
//
// Call order within ContainerInit:
//   1. All setup (mounts, pivot_root, etc.)  — can use any syscall
//   2. applySeccomp()                        ← here
//   3. syscall.Exec(binary, …)              — filter inherited by the user process
//
// Because seccomp filters survive execve(2), the user's process is
// restricted from the moment it starts running.
func applySeccomp(debug bool) error {
	// Step 1: PR_SET_NO_NEW_PRIVS
	//
	// Prevents the exec'd binary from gaining privileges via set-uid bits or
	// file capabilities.  Also enables installing a seccomp filter without
	// needing CAP_SYS_ADMIN.  Once set, cannot be unset.
	if _, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL,
		prSetNoNewPrivs, 1, 0); errno != 0 {
		return fmt.Errorf("prctl(PR_SET_NO_NEW_PRIVS): %w", errno)
	}

	// Step 2: install the BPF filter.
	insns := buildFilter()
	prog := bpfProg{
		len:    uint16(len(insns)),
		filter: &insns[0],
	}

	// prctl(PR_SET_SECCOMP, SECCOMP_MODE_FILTER, *sock_fprog)
	if _, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL,
		syscall.PR_SET_SECCOMP,
		seccompModeFilter,
		uintptr(unsafe.Pointer(&prog))); errno != 0 {
		return fmt.Errorf("prctl(PR_SET_SECCOMP): %w", errno)
	}

	if debug {
		fmt.Printf("[init] seccomp: installed BPF filter (%d instructions, %d blocked syscalls)\n",
			len(insns), len(blockedSyscalls))
	}
	return nil
}
