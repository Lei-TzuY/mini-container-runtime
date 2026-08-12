# Mini Docker & Container Runtime (`minictl`)

A minimal, educational Linux container runtime written in Go from scratch. `minictl` directly invokes Linux kernel syscalls (`clone`, `pivot_root`, `setns`, `mount`, `prctl`) to implement process isolation, namespaces, cgroups v2 resource controls, OverlayFS copy-on-write storage, veth bridge networking, port mapping via iptables DNAT, Linux capabilities management, HTTP OCI registry image pulling, multi-container orchestration (`minictl compose`), dynamic resource updates (`minictl update`), real-time event streaming (`minictl events`), bidirectional container file transfer (`minictl cp`), health checking, and BPF seccomp syscall filtering without depending on Docker, containerd, or runc internally.

---

## 🏛️ Architecture & Syscall Mapping

```
                       ┌──────────────────────────────┐
                       │          minictl CLI         │
                       └──────────────┬───────────────┘
                                      │
              ┌───────────────────────┴───────────────────────┐
              ▼                                               ▼
   Stage 1: Parent Process                          Stage 2: Container Init
   • clone(CLONE_NEWPID|NEWUTS|NEWNS|...)           • Wait on sync pipe from parent
   • Apply cgroups v2 limits & cpus quota           • sethostname(2) in UTS netns
   • Create veth-h<pid> ↔ eth0 pair                 • Mount OverlayFS (--overlay)
   • Configure iptables DNAT (-p)                   • Mount /proc, /sys, /dev/pts
   • Move veth-peer to child netns                  • Bind mount volumes (-v)
   • Unblock child via sync pipe                    • pivot_root(2) into rootfs
   • Wait for child exit & cleanup NAT              • Remount root read-only (--read-only)
                                                    • chdir(2) to workdir (-w)
                                                    • Drop capabilities (PR_CAPBSET_DROP)
                                                    • Install seccomp BPF filter
                                                    • Inject env vars (-e) & execve(2)
```

### Key Subsystems & Syscalls Used

| Feature | Mechanism / Syscall | Purpose |
| :--- | :--- | :--- |
| **PID Namespace** | `clone(CLONE_NEWPID)` | Isolated PID number space; container payload becomes PID 1 inside. |
| **UTS Namespace** | `clone(CLONE_NEWUTS)` + `sethostname(2)` | Isolated hostname & NIS domain name. |
| **Mount Namespace** | `clone(CLONE_NEWNS)` + `mount(2)` | Isolated mount table; host mounts remain hidden. |
| **Network Namespace** | `clone(CLONE_NEWNET)` + Netlink sockets | Isolated network stack (lo interface + veth pair). |
| **IPC Namespace** | `clone(CLONE_NEWIPC)` | Isolated System V IPC & POSIX message queues. |
| **User Namespace** | `clone(CLONE_NEWUSER)` + `uid_map` / `gid_map` | Maps host UID to container root (0/0); enables unprivileged containers. |
| **RootFS Isolation** | `pivot_root(2)` (with `chroot(2)` fallback) | Swaps `/` to new rootfs and unmounts host root via `MNT_DETACH`. |
| **OverlayFS (CoW)** | `mount -t overlay ...` | Merges read-only image layers with a per-container writable `upper` directory. |
| **Resource Limits** | Cgroups v2 (`/sys/fs/cgroup/...`) | Hard memory limits, fractional CPU quotas (`cpu.max`), CPU weights, and PIDs limit. |
| **Dynamic Updates** | Cgroups v2 `/sys/fs/cgroup` | Dynamically modifies memory (`memory.max`) and CPU (`cpu.max`) quotas on the fly. |
| **Process Freezer** | Cgroup v2 `cgroup.freeze` | Atomic pause/unpause of container processes without signal leakage. |
| **Container Exec** | `setns(2)` + `chroot(2)` | Attaches to an existing container's namespaces by host PID. |
| **Capabilities Control**| `prctl(PR_CAPBSET_DROP)` | Drops specific privileges (`CAP_SYS_ADMIN`, `CAP_NET_RAW`) from bounding set. |
| **OCI Image Pulling** | HTTP REST API v2 | Downloads image manifests and layer blobs directly from Docker Hub (`minictl pull`). |
| **Mini Compose** | Declarative JSON Runner | Orchestrates multi-container applications (`minictl compose up -f compose.json`). |
| **Event Audit Stream**| Real-time Event Logger | Streams container lifecycle events (`minictl events -f`). |
| **Container File Copy** | Bidirectional File Transfer | Copies files/directories between host and container (`minictl cp`). |
| **Health Checking** | State & Exec Evaluator | Evaluates container health status (`starting` -> `healthy` / `unhealthy`). |
| **Custom Networks** | `ip link add type bridge` | Creates and manages custom Layer 2 Linux bridge networks. |
| **Port Mapping** | `iptables` DNAT | Forwards host ports to container private IP (172.20.0.2). |
| **Seccomp Security** | `prctl(PR_SET_SECCOMP)` + cBPF | Restricts dangerous syscalls (`ptrace`, `kexec_load`, `init_module`, etc.). |
| **Dynamic IPAM**     | Subnet Lease Pool Allocator| Dynamically allocates & recycles container IP addresses across bridge networks. |
| **Filesystem Diff** | OverlayFS Upper Inspector | Audits filesystem mutations (`A` added, `C` changed, `D` deleted) (`minictl diff`). |
| **Prometheus Exporter**| Metrics Endpoint Exporter| Exposes `/v1/metrics` REST endpoint formatted for Prometheus metrics scrapers. |
| **OCI Image Push**   | Tarball & Manifest Packager| Packages rootfs layers and OCI manifest for image registry upload (`minictl push`). |
| **REST API Daemon**  | HTTP over Unix/TCP Socket | Runs background engine server listening on `/tmp/minictl.sock` or `:2375` (`minictl daemon`). |
| **Interactive PTY**   | Master/Slave PTY Allocator | Provides raw terminal mode pass-through & stream piping for interactive sessions (`-it`). |
| **Health Supervisor**| Periodic Background Evaluator | Background worker evaluating container health command status (`healthy`/`unhealthy`). |
| **Dockerfile Builder**| AST Parser & Layer Executor| Builds container images step-by-step from Dockerfile (`minictl build`). |
| **Image Tag & Store**| Local Manifest Index | Manages local image repositories, tags, IDs, and disk sizes (`minictl images/tag/rmi`). |
| **Named Volumes**   | Persistent Data Driver | Manages named data volumes and automatic volume binding (`minictl volume`). |
| **Container DNS**    | Network Hosts Injector | Auto-registers container hostnames & IPs for service discovery inside bridge networks. |
| **Container Export** | `tar.Writer` | Packages container rootfs into `.tar` / `.tar.gz` (`minictl export`). |
| **Container Commit** | Metadata snapshot | Commits container filesystem as new local image (`minictl commit`). |
| **Container Top** | `/proc/<pid>/task` parsing | Displays tasks/threads running inside the container (`minictl top`). |

---

## 🚀 Requirements & Environment Setup

Linux kernel features (namespaces, cgroups, pivot_root) require a Linux kernel environment.

### Option A: WSL 2 (Windows 10 / 11) — Recommended
1. Open PowerShell as Administrator:
   ```powershell
   wsl --install
   ```
2. Restart your computer and open Ubuntu from the Start menu.
3. Install Go:
   ```bash
   sudo apt update && sudo apt install -y golang-go
   ```

### Option B: Native Linux / Cloud VM / VirtualBox
Install Go (v1.21+):
```bash
sudo apt update && sudo apt install -y golang-go make wget
```

---

## 🛠️ Build & Usage

### 1. Build `minictl`
```bash
make build
# or manually:
go build -o build/minictl ./cmd/minictl
```

### 2. Prepare RootFS Image
Pull an Alpine Linux image directly from Docker Hub:
```bash
./build/minictl pull alpine:3.19 ./rootfs
```

Or download and unpack an Alpine Linux minirootfs manually:
```bash
make rootfs
# or manually:
./build/minictl unpack alpine-minirootfs-3.19.0-x86_64.tar.gz ./rootfs
```

---

## 💻 CLI Command Reference

### `minictl update` — Dynamic Resource Updates
```bash
# Dynamically change memory limit to 128MB and CPU quota to 1.5 CPUs on running container
sudo ./build/minictl update --memory 128m --cpus 1.5 <container-id>
```

### `minictl events` — Real-time Lifecycle Audit Stream
```bash
# Stream real-time container events (create, start, exec, pause, stop, die, destroy)
./build/minictl events -f
```

### `minictl cp` — Bidirectional Container File Transfer
```bash
# Copy file from host into container
./build/minictl cp /tmp/config.json <container-id>:/etc/config.json

# Copy file from container out to host
./build/minictl cp <container-id>:/var/log/app.log /tmp/app.log
```

### `minictl compose up` — Multi-container Orchestration
```bash
sudo ./build/minictl compose up -f compose.json
```

### `minictl pull` — Download Images from Docker Hub
```bash
./build/minictl pull alpine:3.19 ./rootfs-alpine
./build/minictl pull ubuntu:22.04 ./rootfs-ubuntu
```

### `minictl run` — Launch a Container
```bash
# Basic shell with OverlayFS layer isolation, environment variables & working directory
sudo ./build/minictl run --overlay -w /app -e MODE=production ./rootfs /bin/sh

# Resource limits (Memory, CPU quota 50%, PIDs limit) & custom hostname
sudo ./build/minictl run \
  --hostname demo-box \
  --memory 64m \
  --cpus 0.5 \
  --pids-limit 32 \
  ./rootfs /bin/sh

# Security: Drop CAP_SYS_ADMIN & CAP_NET_RAW capabilities + Seccomp BPF filter
sudo ./build/minictl run \
  --cap-drop CAP_SYS_ADMIN \
  --cap-drop CAP_NET_RAW \
  --seccomp \
  ./rootfs /bin/sh

# Bind volumes (-v host:container[:ro])
sudo ./build/minictl run \
  -v /tmp/data:/data \
  -v /etc/hosts:/etc/hosts:ro \
  ./rootfs /bin/sh

# Bridge networking & Port mapping (-p hostPort:containerPort)
sudo ./build/minictl run --bridge -p 8080:80 ./rootfs /bin/nc -l -p 80
```

### Management Commands
```bash
# List containers
./build/minictl ps [-a]

# Inspect JSON metadata
./build/minictl inspect <id>

# View process list inside container
./build/minictl top <id>

# Manage custom bridge networks
sudo ./build/minictl network create demo-net 172.28.0.1/24
./build/minictl network ls
sudo ./build/minictl network rm demo-net

# Execute command inside running container
sudo ./build/minictl exec <id> /bin/sh

# Export container rootfs to a tarball archive
./build/minictl export <id> container-backup.tar.gz

# Commit container to new local image
./build/minictl commit <id> my-custom-app:v1

# View live resource metrics (cgroup v2)
./build/minictl stats [id]

# View container logs
./build/minictl logs [-f] [--tail n] <id>

# Pause / Unpause container processes (cgroup freezer)
sudo ./build/minictl pause <id>
sudo ./build/minictl unpause <id>

# Gracefully stop container (SIGTERM -> SIGKILL)
sudo ./build/minictl stop -t 5 <id>

# Force kill container (SIGKILL)
./build/minictl kill <id>

# Remove container or prune stopped containers
./build/minictl rm <id>
./build/minictl prune
```

---

## 🧪 Testing

Run all unit tests across state store, image unpacking, compose parsing, health check evaluation, container cp transfer, events stream, OCI image ref parsing, export round-tripping, safe-path traversal protection, port parsing, and platform stubs:
```bash
make test
# or manually:
go test -v ./...
```

---

## 🔬 Learning Highlights & Debugging

To observe raw syscall execution and namespace creation in real-time, set `MINICONTAINER_DEBUG=1`:
```bash
sudo MINICONTAINER_DEBUG=1 ./build/minictl run --overlay --cap-drop CAP_SYS_ADMIN ./rootfs /bin/sh
```
Output log trace:
```text
[parent] spawning child with new namespaces
[parent] child started, PID=14205
[cgroup] using cgroup v2 (unified hierarchy)
[cgroup v2] cpu.max = 50000 100000
[parent] veth: host side veth-h14205 ready (172.20.0.1/24)
[init] running inside new namespaces
[init] received sync signal from parent
[init] mount namespace propagation set to private
[init] overlayfs mounted (./rootfs -> /tmp/minicontainer-overlay-1234/merged)
[init] hostname set to "minicontainer"
[init] /proc mounted
[init] dropped capability CAP_SYS_ADMIN (21)
[init] pivot_root complete
[init] exec: /bin/sh []
```
