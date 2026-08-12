//go:build linux && arm64

package container

// blockedSyscalls for AArch64 (arm64).
// arm64 uses the generic syscall table (include/uapi/asm-generic/unistd.h).
// Numbers verified against Linux 6.x kernel headers.
//
// Note: create_module, iopl, ioperm do not exist in the arm64 ABI
// (they are x86-specific or have been removed), so they are omitted.
var blockedSyscalls = []uint32{
	104, // kexec_load        replace the running kernel
	294, // kexec_file_load   same but from a file descriptor
	117, // ptrace            trace / inject into other processes
	142, // reboot            reboot / power-off the system
	116, // syslog            read the kernel message buffer
	105, // init_module       load a kernel module from memory
	273, // finit_module      load a kernel module from fd
	106, // delete_module     remove a kernel module
	170, // settimeofday      set the system clock
	112, // clock_settime     set a POSIX clock
	404, // clock_settime64   64-bit variant (newer kernels)
	40,  // mount             mount a filesystem
	39,  // umount2           unmount a filesystem
	41,  // pivot_root        change root filesystem
	224, // swapon            enable a swap device
	225, // swapoff           disable a swap device
	89,  // acct              enable/disable process accounting
	217, // add_key           add a key to the kernel keyring
	218, // request_key       request a kernel key
	219, // keyctl            manipulate the kernel keyring
	280, // bpf               load BPF programs (seccomp bypass)
	241, // perf_event_open   performance counters (info leak)
	270, // process_vm_readv  read another process's memory
	271, // process_vm_writev write into another process
	264, // open_by_handle_at bypass chroot via NFS handle
	262, // fanotify_init     filesystem event notification
	282, // userfaultfd       user-space page-fault (exploit primitive)
	97,  // unshare           re-create namespaces inside container
}
