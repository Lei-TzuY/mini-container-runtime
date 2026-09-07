package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"minicontainer/internal/container"
)

type ociSeccompConfig struct {
	DefaultAction string   `json:"defaultAction"`
	Architectures []string `json:"architectures,omitempty"`
	Syscalls      []struct {
		Names    []string          `json:"names"`
		Action   string            `json:"action"`
		ErrnoRet *uint             `json:"errnoRet,omitempty"`
		Args     []json.RawMessage `json:"args,omitempty"`
	} `json:"syscalls,omitempty"`
}

type ociCapabilitiesConfig struct {
	Bounding    *[]string `json:"bounding,omitempty"`
	Effective   *[]string `json:"effective,omitempty"`
	Permitted   *[]string `json:"permitted,omitempty"`
	Inheritable *[]string `json:"inheritable,omitempty"`
	Ambient     *[]string `json:"ambient,omitempty"`
}

type ociBundleConfig struct {
	OCIVersion string `json:"ociVersion"`
	Root       struct {
		Path     string `json:"path"`
		Readonly bool   `json:"readonly,omitempty"`
	} `json:"root"`
	Process struct {
		Terminal        bool                   `json:"terminal,omitempty"`
		Args            []string               `json:"args"`
		Env             []string               `json:"env,omitempty"`
		Cwd             string                 `json:"cwd"`
		NoNewPrivileges bool                   `json:"noNewPrivileges,omitempty"`
		Capabilities    *ociCapabilitiesConfig `json:"capabilities,omitempty"`
	} `json:"process"`
	Hostname string `json:"hostname,omitempty"`
	Mounts   []struct {
		Destination string   `json:"destination"`
		Type        string   `json:"type"`
		Source      string   `json:"source"`
		Options     []string `json:"options,omitempty"`
	} `json:"mounts,omitempty"`
	Linux *struct {
		Namespaces []struct {
			Type string `json:"type"`
			Path string `json:"path,omitempty"`
		} `json:"namespaces,omitempty"`
		Resources *struct {
			Memory *struct {
				Limit *int64 `json:"limit,omitempty"`
			} `json:"memory,omitempty"`
			CPU *struct {
				Shares *uint64 `json:"shares,omitempty"`
				Quota  *int64  `json:"quota,omitempty"`
				Period *uint64 `json:"period,omitempty"`
			} `json:"cpu,omitempty"`
			Pids *struct {
				Limit int64 `json:"limit"`
			} `json:"pids,omitempty"`
		} `json:"resources,omitempty"`
		Seccomp *ociSeccompConfig `json:"seccomp,omitempty"`
	} `json:"linux,omitempty"`
}

func loadOCIBundle(bundle string) (container.Config, error) {
	abs, err := filepath.Abs(bundle)
	if err != nil {
		return container.Config{}, fmt.Errorf("resolve bundle: %w", err)
	}
	data, err := os.ReadFile(filepath.Join(abs, "config.json"))
	if err != nil {
		return container.Config{}, fmt.Errorf("read config.json: %w", err)
	}
	var spec ociBundleConfig
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&spec); err != nil {
		return container.Config{}, fmt.Errorf("decode config.json: %w", err)
	}
	if spec.OCIVersion == "" {
		return container.Config{}, fmt.Errorf("ociVersion is required")
	}
	if spec.Root.Path == "" {
		return container.Config{}, fmt.Errorf("root.path is required")
	}
	if filepath.IsAbs(spec.Root.Path) {
		return container.Config{}, fmt.Errorf("root.path must be relative to bundle")
	}
	if len(spec.Process.Args) == 0 || spec.Process.Args[0] == "" {
		return container.Config{}, fmt.Errorf("process.args must contain an executable")
	}
	if spec.Process.Cwd == "" || !filepath.IsAbs(spec.Process.Cwd) {
		return container.Config{}, fmt.Errorf("process.cwd must be absolute")
	}
	if spec.Process.Terminal {
		return container.Config{}, fmt.Errorf("process.terminal is not supported")
	}
	for _, e := range spec.Process.Env {
		if i := strings.IndexByte(e, '='); i <= 0 {
			return container.Config{}, fmt.Errorf("invalid process.env entry %q", e)
		}
	}

	capDrop, err := translateOCICapabilities(spec.Process.Capabilities)
	if err != nil {
		return container.Config{}, err
	}
	cfg := container.Config{
		ReadOnly:        spec.Root.Readonly,
		Command:         append([]string(nil), spec.Process.Args...),
		Env:             append([]string(nil), spec.Process.Env...),
		WorkDir:         spec.Process.Cwd,
		Hostname:        spec.Hostname,
		NoNewPrivileges: spec.Process.NoNewPrivileges,
		CapDrop:         capDrop,
	}
	for _, mount := range spec.Mounts {
		volume, err := translateOCIBindMount(mount.Destination, mount.Type, mount.Source, mount.Options)
		if err != nil {
			return container.Config{}, err
		}
		cfg.Volumes = append(cfg.Volumes, volume)
	}
	if spec.Linux != nil {
		seenNamespaces := make(map[string]struct{}, len(spec.Linux.Namespaces))
		for _, ns := range spec.Linux.Namespaces {
			if ns.Path != "" {
				return container.Config{}, fmt.Errorf("joining existing %s namespace is not supported", ns.Type)
			}
			if _, exists := seenNamespaces[ns.Type]; exists {
				return container.Config{}, fmt.Errorf("duplicate linux namespace %q", ns.Type)
			}
			seenNamespaces[ns.Type] = struct{}{}
			switch ns.Type {
			case "pid", "mount", "uts", "ipc", "network":
			case "user":
				cfg.UserNS = true
			case "cgroup":
				if runtime.GOOS != "linux" {
					return container.Config{}, fmt.Errorf("linux cgroup namespace requires linux")
				}
				cfg.CgroupNS = true
			default:
				return container.Config{}, fmt.Errorf("unsupported linux namespace %q", ns.Type)
			}
		}
		if err := applyOCIResources(&cfg, spec.Linux.Resources); err != nil {
			return container.Config{}, err
		}
		seccomp, err := translateOCISeccomp(spec.Linux.Seccomp)
		if err != nil {
			return container.Config{}, err
		}
		cfg.Seccomp = seccomp
	}

	rootfs := filepath.Clean(filepath.Join(abs, spec.Root.Path))
	rel, err := filepath.Rel(abs, rootfs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return container.Config{}, fmt.Errorf("root.path escapes bundle")
	}
	cfg.RootFS = rootfs
	return cfg, nil
}

var ociKnownLinuxCapabilities = []string{
	"CAP_CHOWN", "CAP_DAC_OVERRIDE", "CAP_DAC_READ_SEARCH", "CAP_FOWNER", "CAP_FSETID",
	"CAP_KILL", "CAP_SETGID", "CAP_SETUID", "CAP_SETPCAP", "CAP_LINUX_IMMUTABLE",
	"CAP_NET_BIND_SERVICE", "CAP_NET_BROADCAST", "CAP_NET_ADMIN", "CAP_NET_RAW", "CAP_IPC_LOCK",
	"CAP_IPC_OWNER", "CAP_SYS_MODULE", "CAP_SYS_RAWIO", "CAP_SYS_CHROOT", "CAP_SYS_PTRACE",
	"CAP_SYS_PACCT", "CAP_SYS_ADMIN", "CAP_SYS_BOOT", "CAP_SYS_NICE", "CAP_SYS_RESOURCE",
	"CAP_SYS_TIME", "CAP_SYS_TTY_CONFIG", "CAP_MKNOD", "CAP_LEASE", "CAP_AUDIT_WRITE",
	"CAP_AUDIT_CONTROL", "CAP_SETFCAP", "CAP_MAC_OVERRIDE", "CAP_MAC_ADMIN", "CAP_SYSLOG",
	"CAP_WAKE_ALARM", "CAP_BLOCK_SUSPEND", "CAP_AUDIT_READ", "CAP_PERFMON", "CAP_BPF",
	"CAP_CHECKPOINT_RESTORE",
}

func translateOCICapabilities(caps *ociCapabilitiesConfig) ([]string, error) {
	if caps == nil {
		return nil, nil
	}
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("OCI process capabilities require linux")
	}
	if caps.Effective != nil || caps.Permitted != nil || caps.Inheritable != nil || caps.Ambient != nil {
		return nil, fmt.Errorf("OCI effective/permitted/inheritable/ambient capability sets are not yet representable by the runtime")
	}
	if caps.Bounding == nil {
		return nil, nil
	}

	known := make(map[string]struct{}, len(ociKnownLinuxCapabilities))
	for _, name := range ociKnownLinuxCapabilities {
		known[name] = struct{}{}
	}
	keep := make(map[string]struct{}, len(*caps.Bounding))
	for _, raw := range *caps.Bounding {
		name := strings.ToUpper(strings.TrimSpace(raw))
		if !strings.HasPrefix(name, "CAP_") {
			return nil, fmt.Errorf("invalid OCI capability %q: capability names must use CAP_ prefix", raw)
		}
		if _, ok := known[name]; !ok {
			return nil, fmt.Errorf("unsupported OCI capability %q", raw)
		}
		if _, duplicate := keep[name]; duplicate {
			return nil, fmt.Errorf("duplicate OCI bounding capability %q", raw)
		}
		keep[name] = struct{}{}
	}

	drops := make([]string, 0, len(known)-len(keep))
	for _, name := range ociKnownLinuxCapabilities {
		if _, retained := keep[name]; !retained {
			drops = append(drops, name)
		}
	}
	return drops, nil
}

var builtinSeccompAMD64Syscalls = []string{
	"kexec_load", "kexec_file_load", "ptrace", "reboot", "syslog",
	"init_module", "finit_module", "delete_module", "create_module", "iopl", "ioperm",
	"settimeofday", "clock_settime", "clock_settime64", "mount", "umount2", "pivot_root",
	"swapon", "swapoff", "acct", "add_key", "request_key", "keyctl", "bpf",
	"perf_event_open", "process_vm_readv", "process_vm_writev", "open_by_handle_at",
	"fanotify_init", "userfaultfd", "unshare",
}

func translateOCISeccomp(seccomp *ociSeccompConfig) (bool, error) {
	if seccomp == nil {
		return false, nil
	}
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return false, fmt.Errorf("OCI seccomp profile is currently supported only on linux/amd64")
	}
	if seccomp.DefaultAction != "SCMP_ACT_ALLOW" {
		return false, fmt.Errorf("OCI seccomp defaultAction %q cannot be represented by the runtime built-in profile", seccomp.DefaultAction)
	}
	if len(seccomp.Architectures) > 0 {
		if len(seccomp.Architectures) != 1 || seccomp.Architectures[0] != "SCMP_ARCH_X86_64" {
			return false, fmt.Errorf("OCI seccomp architectures must be exactly [SCMP_ARCH_X86_64]")
		}
	}

	want := make(map[string]struct{}, len(builtinSeccompAMD64Syscalls))
	for _, name := range builtinSeccompAMD64Syscalls {
		want[name] = struct{}{}
	}
	seen := make(map[string]struct{}, len(want))
	for _, rule := range seccomp.Syscalls {
		if rule.Action != "SCMP_ACT_KILL_PROCESS" {
			return false, fmt.Errorf("OCI seccomp syscall action %q cannot be represented by the runtime built-in profile", rule.Action)
		}
		if rule.ErrnoRet != nil || len(rule.Args) != 0 {
			return false, fmt.Errorf("OCI seccomp errno/argument filtering is not supported")
		}
		if len(rule.Names) == 0 {
			return false, fmt.Errorf("OCI seccomp syscall rule has no names")
		}
		for _, name := range rule.Names {
			if _, ok := want[name]; !ok {
				return false, fmt.Errorf("OCI seccomp syscall %q is outside the runtime built-in profile", name)
			}
			if _, duplicate := seen[name]; duplicate {
				return false, fmt.Errorf("duplicate OCI seccomp syscall %q", name)
			}
			seen[name] = struct{}{}
		}
	}
	if len(seen) != len(want) {
		return false, fmt.Errorf("OCI seccomp profile does not exactly match the runtime built-in block list")
	}
	return true, nil
}

func translateOCIBindMount(destination, mountType, source string, options []string) (container.Volume, error) {
	if mountType != "bind" {
		return container.Volume{}, fmt.Errorf("unsupported OCI mount type %q", mountType)
	}
	if !filepath.IsAbs(source) {
		return container.Volume{}, fmt.Errorf("OCI bind mount source %q must be absolute", source)
	}
	if !filepath.IsAbs(destination) || filepath.Clean(destination) == "/" {
		return container.Volume{}, fmt.Errorf("OCI bind mount destination %q must be an absolute path below root", destination)
	}
	readonly := false
	for _, option := range options {
		switch option {
		case "bind", "rbind", "rw":
		case "ro":
			readonly = true
		default:
			return container.Volume{}, fmt.Errorf("unsupported OCI bind mount option %q", option)
		}
	}
	return container.Volume{HostPath: filepath.Clean(source), ContainerPath: filepath.Clean(destination), ReadOnly: readonly}, nil
}

func applyOCIResources(cfg *container.Config, resources *struct {
	Memory *struct {
		Limit *int64 `json:"limit,omitempty"`
	} `json:"memory,omitempty"`
	CPU *struct {
		Shares *uint64 `json:"shares,omitempty"`
		Quota  *int64  `json:"quota,omitempty"`
		Period *uint64 `json:"period,omitempty"`
	} `json:"cpu,omitempty"`
	Pids *struct {
		Limit int64 `json:"limit"`
	} `json:"pids,omitempty"`
}) error {
	if resources == nil {
		return nil
	}
	if resources.Memory != nil && resources.Memory.Limit != nil {
		if *resources.Memory.Limit <= 0 {
			return fmt.Errorf("linux.resources.memory.limit must be greater than zero")
		}
		cfg.Memory = *resources.Memory.Limit
	}
	if resources.Pids != nil {
		if resources.Pids.Limit <= 0 {
			return fmt.Errorf("linux.resources.pids.limit must be greater than zero")
		}
		cfg.PidsLimit = resources.Pids.Limit
	}
	if resources.CPU != nil {
		cpu := resources.CPU
		if cpu.Shares != nil {
			if *cpu.Shares < 2 || *cpu.Shares > 262144 {
				return fmt.Errorf("linux.resources.cpu.shares must be in range 2..262144")
			}
			cfg.CPUWeight = 1 + int64((*cpu.Shares-2)*9999/262142)
		}
		if (cpu.Quota == nil) != (cpu.Period == nil) {
			return fmt.Errorf("linux.resources.cpu.quota and period must be specified together")
		}
		if cpu.Quota != nil {
			if *cpu.Quota == -1 {
				cfg.CPUs = 0
			} else {
				if *cpu.Quota <= 0 || *cpu.Period == 0 {
					return fmt.Errorf("linux.resources.cpu quota and period must be positive, or quota may be -1")
				}
				cfg.CPUs = float64(*cpu.Quota) / float64(*cpu.Period)
			}
		}
	}
	return nil
}
