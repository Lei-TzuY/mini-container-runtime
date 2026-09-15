#!/usr/bin/env bash
set -euo pipefail

minictl="${1:?path to minictl binary is required}"
busybox="${2:?path to static busybox is required}"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "full-stack OCI smoke must run as root" >&2
  exit 1
fi
if [[ ! -x "$minictl" || ! -x "$busybox" ]]; then
  echo "minictl and busybox must both be executable" >&2
  exit 1
fi

work="$(mktemp -d -t minictl-oci-full-stack.XXXXXX)"
cleanup() {
  if [[ -n "${work:-}" && "$work" == /tmp/minictl-oci-full-stack.* ]]; then
    rm -rf -- "$work"
  fi
}
trap cleanup EXIT

bundle="$work/bundle"
rootfs="$bundle/rootfs"
evidence="$work/evidence"
runtime_home="$work/home"
mkdir -p "$rootfs/bin" "$rootfs/dev" "$rootfs/proc" "$rootfs/sys" "$rootfs/tmp" "$rootfs/evidence" "$evidence" "$runtime_home"
install -m 0755 "$busybox" "$rootfs/bin/busybox"
for applet in sh awk cat grep hostname readlink tr; do
  ln -s busybox "$rootfs/bin/$applet"
done

host_mnt_ns="$(readlink /proc/self/ns/mnt)"
host_pid_ns="$(readlink /proc/self/ns/pid)"

EVIDENCE_DIR="$evidence" CONFIG_PATH="$bundle/config.json" python3 - <<'PY'
import json
import os

blocked = [
    "kexec_load", "kexec_file_load", "ptrace", "reboot", "syslog",
    "init_module", "finit_module", "delete_module", "create_module", "iopl", "ioperm",
    "settimeofday", "clock_settime", "clock_settime64", "mount", "umount2", "pivot_root",
    "swapon", "swapoff", "acct", "add_key", "request_key", "keyctl", "bpf",
    "perf_event_open", "process_vm_readv", "process_vm_writev", "open_by_handle_at",
    "fanotify_init", "userfaultfd", "unshare",
]
payload = r"""set -eu
test "$(hostname)" = "conquest-e2e"
readlink /proc/self/ns/mnt > /evidence/mnt.ns
readlink /proc/self/ns/pid > /evidence/pid.ns
readlink /proc/self/ns/cgroup > /evidence/cgroup.ns
tr '\\000' ' ' < /proc/1/cmdline > /evidence/init.cmdline
grep -Eq '^NoNewPrivs:[[:space:]]*1$' /proc/self/status
grep -Eq '^Seccomp:[[:space:]]*2$' /proc/self/status
cg_path="$(awk -F: '$1 == "0" {print $3}' /proc/self/cgroup)"
test -n "$cg_path"
cat "/sys/fs/cgroup${cg_path}/memory.max" > /evidence/memory.max
cat "/sys/fs/cgroup${cg_path}/pids.max" > /evidence/pids.max
printf 'ok\n' > /evidence/result
"""
config = {
    "ociVersion": "1.1.0",
    "root": {"path": "rootfs"},
    "process": {
        "args": ["/bin/sh", "-c", payload],
        "env": ["PATH=/bin"],
        "cwd": "/",
    },
    "hostname": "conquest-e2e",
    "mounts": [{
        "destination": "/evidence",
        "type": "bind",
        "source": os.environ["EVIDENCE_DIR"],
        "options": ["rbind", "rw"],
    }],
    "linux": {
        "namespaces": [
            {"type": "pid"},
            {"type": "mount"},
            {"type": "uts"},
            {"type": "ipc"},
            {"type": "network"},
        ],
        "resources": {
            "memory": {"limit": 134217728},
            "pids": {"limit": 32},
        },
        "seccomp": {
            "defaultAction": "SCMP_ACT_ALLOW",
            "architectures": ["SCMP_ARCH_X86_64"],
            "syscalls": [{
                "names": blocked,
                "action": "SCMP_ACT_KILL_PROCESS",
            }],
        },
    },
}
with open(os.environ["CONFIG_PATH"], "w", encoding="utf-8") as stream:
    json.dump(config, stream)
PY

HOME="$runtime_home" MINICONTAINER_DEBUG=1 "$minictl" oci-run "$bundle"

require_file() {
  local path="$1"
  [[ -s "$path" ]] || {
    echo "missing full-stack evidence: $path" >&2
    exit 1
  }
}
for name in result mnt.ns pid.ns cgroup.ns init.cmdline memory.max pids.max; do
  require_file "$evidence/$name"
done

[[ "$(<"$evidence/result")" == "ok" ]] || {
  echo "payload did not complete its evidence contract" >&2
  exit 1
}
[[ "$(<"$evidence/mnt.ns")" != "$host_mnt_ns" ]] || {
  echo "mount namespace was not isolated" >&2
  exit 1
}
[[ "$(<"$evidence/pid.ns")" != "$host_pid_ns" ]] || {
  echo "PID namespace was not isolated" >&2
  exit 1
}
grep -q '__minicontainer-init-supervisor' "$evidence/init.cmdline" || {
  echo "PID 1 was not the minictl init supervisor" >&2
  exit 1
}
[[ "$(<"$evidence/memory.max")" == "134217728" ]] || {
  echo "memory.max was not applied: $(<"$evidence/memory.max")" >&2
  exit 1
}
[[ "$(<"$evidence/pids.max")" == "32" ]] || {
  echo "pids.max was not applied: $(<"$evidence/pids.max")" >&2
  exit 1
}

echo "full-stack OCI isolation evidence passed"
