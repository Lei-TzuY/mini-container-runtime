//go:build linux && amd64

package container

// blockedSyscalls for x86-64 (amd64).
// Numbers sourced from <asm/unistd_64.h> in the Linux kernel.
var blockedSyscalls = []uint32{
	246, // kexec_load        replace the running kernel
	320, // kexec_file_load   same but from a file descriptor
	101, // ptrace            trace / inject into other processes
	169, // reboot            reboot / power-off the system
	103, // syslog            read the kernel message buffer
	175, // init_module       load a kernel module from memory
	313, // finit_module      load a kernel module from fd
	176, // delete_module     remove a kernel module
	174, // create_module     (obsolete) create loadable module entry
	172, // iopl              change process I/O privilege level
	173, // ioperm            enable access to I/O ports
	164, // settimeofday      set the system clock
	227, // clock_settime     set a POSIX clock
	404, // clock_settime64   64-bit variant (newer kernels)
	165, // mount             mount a filesystem
	166, // umount2           unmount a filesystem
	155, // pivot_root        change root filesystem
	167, // swapon            enable a swap device
	168, // swapoff           disable a swap device
	163, // acct              enable/disable process accounting
	248, // add_key           add a key to the kernel keyring
	249, // request_key       request a kernel key
	250, // keyctl            manipulate the kernel keyring
	321, // bpf               load BPF programs (seccomp bypass)
	298, // perf_event_open   performance counters (info leak)
	310, // process_vm_readv  read another process's memory
	311, // process_vm_writev write into another process
	304, // open_by_handle_at bypass chroot via NFS handle
	300, // fanotify_init     filesystem event notification
	323, // userfaultfd       user-space page-fault (exploit primitive)
	272, // unshare           re-create namespaces inside container
}
