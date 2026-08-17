# Mini Container Runtime (`minictl`)

An educational Linux container runtime written in Go from scratch. `minictl` uses Linux kernel primitives directly — namespaces, cgroups v2, mounts, `pivot_root`, veth networking, capabilities and seccomp — without using Docker, containerd or runc as its runtime.

> **Goal:** make the mechanics behind containers visible and hackable, not compete with production runtimes.

## What it actually implements

The core runtime path covers the pieces that make a Linux container a container:

- **Isolation:** PID, UTS, mount, network, IPC and user namespaces
- **Filesystem:** rootfs isolation with `pivot_root`, bind mounts and OverlayFS copy-on-write layers
- **Resources:** cgroups v2 memory, CPU and PID controls, plus runtime updates
- **Networking:** network namespaces, veth pairs, Linux bridges and host-port forwarding
- **Images:** OCI / Docker Registry HTTP pulling and layer extraction
- **Security:** Linux capability dropping and seccomp BPF filtering
- **Lifecycle:** run, exec, stop/kill, pause/unpause, inspect, logs, events and cleanup
- **Orchestration:** a small declarative compose-style runner for multi-container experiments

The repository also contains a larger set of inspection and observability helpers. Those are secondary to the runtime core above; the README intentionally keeps them out of the opening pitch so the important mechanisms are easy to audit.

## Architecture

```text
                         minictl CLI
                             |
              +--------------+--------------+
              |                             |
              v                             v
       Parent / runtime                Container init
       ----------------                --------------
       clone namespaces                wait for setup
       configure cgroups               set hostname
       create veth pair                mount rootfs
       configure bridge/NAT            mount proc/sys/dev
       prepare rootfs                   pivot_root
       apply policy                     drop capabilities
       release child                    install seccomp
       track / clean up                 exec payload
```

The runtime directly reaches kernel interfaces such as:

```text
clone / setns / mount / pivot_root / prctl
/sys/fs/cgroup/*
netlink + veth / bridge
iptables DNAT
OCI Registry HTTP API
```

## Why this project is useful

A normal `docker run` hides a lot of machinery. `minictl` keeps that machinery in one readable codebase so you can trace questions such as:

- When does a process enter its PID namespace?
- How is a container root filesystem separated from the host?
- Where are memory and CPU limits enforced?
- How does a veth pair connect an isolated network namespace back to the host?
- What does an OCI image pull actually download and unpack?
- Which privileges remain after capabilities and seccomp are applied?

That makes it useful as a systems-programming project and as a reference for learning Linux container internals.

## Repository structure

```text
cmd/minictl/        CLI and command wiring
internal/           runtime subsystems
.github/workflows/  CI
Makefile             build / test helpers
compose.json         small orchestration example
```

The implementation is split into focused `internal/` packages rather than one monolithic runtime file. Tests live next to many of those packages and GitHub Actions exercises the repository automatically.

## Build and test

Linux is required for the full runtime because the implementation depends on Linux namespaces, cgroups, OverlayFS and networking primitives.

```sh
go build ./...
go test ./...
```

Or use the provided Makefile targets where appropriate.

Some integration paths require root privileges or host capabilities to create namespaces, mounts, bridges and iptables rules.

## Minimal walkthrough

Build the CLI:

```sh
go build -o minictl ./cmd/minictl
```

Then use the CLI to pull an image / prepare a rootfs and launch isolated processes according to the commands exposed by the current build.

The important part is not the Docker-like command surface; it is what happens underneath it: namespace creation, rootfs setup, resource control, networking, privilege reduction and process execution.

## Core subsystem map

| Subsystem | Linux mechanism | Responsibility |
| --- | --- | --- |
| Process isolation | PID namespace | independent PID view / container init |
| Hostname isolation | UTS namespace | isolated hostname |
| Filesystem isolation | mount namespace + `pivot_root` | separate mount table and root |
| Network isolation | network namespace | independent interfaces / routes |
| Rootfs layers | OverlayFS | copy-on-write image + writable layer |
| Resource limits | cgroups v2 | memory / CPU / PIDs |
| Exec | `setns` | join an existing container's namespaces |
| Networking | veth + bridge | connect container namespace to host |
| Port mapping | iptables DNAT | forward host traffic to container |
| Image distribution | OCI Registry API | fetch manifests, configs and layers |
| Capabilities | `prctl` / capability sets | reduce privileged operations |
| Syscall policy | seccomp BPF | deny selected syscall classes |

## Scope and limitations

`minictl` is deliberately educational. It does **not** claim Docker/containerd/runc compatibility, production-grade security hardening, or complete OCI runtime-spec coverage.

The codebase favors explicit mechanisms and experiments over abstraction layers. Some features depend on the host kernel, cgroup configuration, filesystem support and privileges, so behavior can vary across Linux environments.

When evaluating the project, the most meaningful parts are the runtime path, tests and kernel interactions — not the raw number of CLI flags.

## License

MIT — see [LICENSE](LICENSE).
