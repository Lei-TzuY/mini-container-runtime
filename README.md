# mini-container-runtime (`minictl`)

A research and teaching container runtime written in Go. The project exercises
Linux isolation primitives directly so their ordering, failure modes, and
cleanup contracts can be inspected without hiding them behind Docker,
containerd, or runc.

This is not a production container engine or a drop-in Docker replacement.

## Current engineering focus

The core runtime path is deliberately smaller than the command surface:

1. create PID, UTS, mount, network, IPC, and optional user/cgroup namespaces;
2. prepare the root filesystem and mount policy;
3. attach cgroup v2 limits and optional bridge/veth networking;
4. enter the container as a PID-1 supervisor;
5. apply credentials, capabilities, `no_new_privileges`, and seccomp policy;
6. execute the payload, forward signals, reap descendants, and clean up owned
   host resources.

The repository also contains many experimental OCI, image, logging, DNS,
cgroup-telemetry, and inspection utilities. Their presence is not a claim of
Docker compatibility, complete OCI conformance, or production hardening.
Use `minictl help` and the focused package tests to inspect the implemented
surface.

## Verification contract

A green check only proves the layer that actually ran.

| Evidence level | What runs | What it supports |
| --- | --- | --- |
| Unprivileged CI | `go vet ./...` and `go test ./...` | parsing, policy, state, fail-closed behavior, process supervision, and model tests |
| Privileged kernel CI | a named subsystem acceptance set as root on a disposable Ubuntu runner | selected live namespace, cgroup-namespace, devpts, seccomp, bridge, veth, and descendant-cleanup regressions |
| Privileged full-stack smoke | one static-rootfs `minictl oci-run` from bundle admission through payload exit | the tested PID/mount namespaces, bind mount, cgroup limits, PID-1, no-new-privileges, and seccomp combination |
| Not yet claimed | no multi-kernel matrix, hostile multi-tenant audit, reboot test, or all-options end-to-end matrix | production security, complete OCI/Docker conformance, and every feature combination |

The privileged job runs only the named kernel acceptance set, records `go test -json`
output, and fails if any of these
critical live-kernel tests are skipped or do not pass:

- `TestBuildCloneFlagsCreatesDistinctCgroupNamespace`
- `TestContainerInitSupervisorDrainsEscapedSessionDescendant`
- `TestPrivateDevptsMountKernel`
- `TestSeccompLiveProcessAllowsNativeABIAndKillsBlockedSyscall`
- `TestInspectBridgeIPv4OwnedKernel`
- `TestAttachVethHostToOwnedBridgeKernel`

The same privileged job also builds `minictl`, creates a static BusyBox rootfs,
and executes one real OCI bundle through `minictl oci-run`. The payload writes
evidence through a bind mount; the host then verifies distinct PID and mount namespace identities, the PID-1 supervisor,
applied `memory.max` and
`pids.max`, and active no-new-privileges/seccomp state.

This is stronger than treating mocked `ip`, `mount`, cgroup files, or a
host-side shell closure as kernel evidence. It proves one deliberately bounded
composition on the Ubuntu runner, not complete OCI/Docker conformance or
portability across kernels.

## Architecture

`minictl run` divides responsibility between a host-side parent and a
re-executed container init:

- The parent owns admission, namespace creation, cgroups, host networking,
  state publication, waiting, and cleanup.
- The container init owns mount propagation, rootfs transition, container
  mounts, hostname, security policy, and payload supervision.
- The payload is a child of the init supervisor. The supervisor forwards
  signals, preserves the payload exit status, reaps orphans, and terminates
  descendants that escape the payload process group.

Important implementation areas:

| Area | Package |
| --- | --- |
| Namespace clone configuration | `internal/ns` |
| Runtime admission and lifecycle | `internal/container` |
| Rootfs and mount policy | `internal/rootfs` |
| Cgroup v2 controls | `internal/cgroups` |
| Bridge/veth/port ownership | `internal/network` |
| OCI/image handling | `internal/image`, `internal/registry` |
| Persistent runtime state | `internal/state` |
| PID-1 wrapper and CLI | `cmd/minictl` |

## Requirements

- Linux
- Go version declared in `go.mod`
- a disposable VM or development host for privileged experiments
- `iproute2` and `iptables` for bridge/port paths
- a cgroup v2 host for resource-control paths

Do not run privileged experiments on a host whose network, mount, or cgroup
state you cannot safely discard.

## Build and test

```bash
go build -o build/minictl ./cmd/minictl
go vet ./...
go test ./...
```

To reproduce the privileged CI layer, use the exact `-run` acceptance regex
in [`.github/workflows/tests.yml`](.github/workflows/tests.yml) on a disposable
Linux host. Running the entire suite as root is intentionally not the contract:
some unprivileged credential tests depend on the invoking user's identity.

Individual kernel tests can be run with `-run` while developing one subsystem. A skipped kernel test is an environment result, not passing
evidence.

## Bounded run example

Prepare a rootfs using the image path or tarball workflow supported by the
CLI, then run a small payload:

```bash
sudo ./build/minictl run \
  --hostname demo-box \
  --memory 64m \
  --cpus 0.5 \
  --pids-limit 32 \
  --cap-drop CAP_NET_RAW \
  --seccomp \
  ./rootfs /bin/sh
```

For syscall-level tracing during development:

```bash
sudo MINICONTAINER_DEBUG=1 ./build/minictl run ./rootfs /bin/true
```

## Direction for new work

New changes should strengthen a coherent runtime boundary rather than add
another isolated flag or reporting variant.

A feature PR should include:

1. one bounded user-visible contract;
2. fail-closed behavior for invalid or unsupported host state;
3. a deterministic unit or process regression;
4. live-kernel evidence when the claim depends on kernel behavior;
5. cleanup and ownership tests for any host resource it creates;
6. an explicit statement of what the test does not prove.

Prefer extending an existing package API over copying command logic. Prefer one
vertical slice across admission, execution, cleanup, and evidence over many
small capability-only milestones.
