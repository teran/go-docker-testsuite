# Libvirtd

Runs a [libvirtd](https://libvirt.org/) container (KVM/QEMU virtualization
manager) for integration testing and returns a connected
[`go-libvirt`](https://github.com/digitalocean/go-libvirt) client. The client
exposes the full libvirt API — virtual machine (domain) lifecycle, storage
pools and volumes, virtual networks, snapshots, and more.

libvirtd is configured to listen on **TCP** (port 16509), which is exposed to
the host so the test connects over TCP.

The client interface provides `Client()`, `Addr()`, `HasKVM()`, and
`Close(ctx)`.

## Privileges and requirements

Running libvirtd + QEMU inside a container needs several things. The wrapper
sets them up automatically; this section documents what and why.

### TCP (default)

The container mounts a `libvirtd.conf` with:

```text
listen_tcp = 1
listen_tls = 0
auth_tcp = "none"
listen_addr = "0.0.0.0"
```

- `listen_tls = 0` is required — otherwise libvirtd tries to set up TLS and
  aborts when no CA certificate is present.
- `auth_tcp = "none"` disables authentication (fine for an ephemeral test
  container).

The port `16509` is exposed to the host and the test connects over TCP.

### Capabilities

By default the container runs in a **least-privilege** configuration:
`--cap-drop=ALL` plus only the capabilities QEMU/libvirtd need:

```text
NET_ADMIN        NET_RAW        NET_BIND_SERVICE
DAC_OVERRIDE     DAC_READ_SEARCH
SYS_NICE         SYS_RESOURCE   SYS_PTRACE
MKNOD            CHOWN          SETUID         SETGID
FOWNER           FSETID         KILL           SETPCAP
IPC_LOCK         AUDIT_WRITE
```

Docker's default seccomp profile is relaxed (`seccomp=unconfined`) because QEMU
needs syscalls it blocks. This config supports: connect, enumeration, and
storage pool/volume CRUD.

### Privileged mode (`WithPrivileged()`)

Booting VMs and creating libvirt virtual networks require more than the
least-privilege set:

- **cgroup** — launching QEMU creates cgroups under `/sys/fs/cgroup`; this
  needs a privileged container and a `/sys/fs/cgroup` mount (the wrapper adds
  it in privileged mode).
- **`/proc/sys`** — creating a network bridge writes
  `/proc/sys/net/ipv6/conf/<bridge>/disable_ipv6`, which is read-only outside
  a privileged container.

Pass `libvirtd.WithPrivileged()` to `New`/`NewWithImage` for VM boot and
network tests:

```go
app, err := libvirtd.New(ctx, libvirtd.WithPrivileged())
```

### Device passthrough

`/dev/kvm` and `/dev/net/tun` are passed through **only if they exist on the
host** (a missing device never fails container creation):

- **`/dev/kvm`** — hardware acceleration. When absent, QEMU falls back to
  software (TCG) emulation — slower but functional. `HasKVM()` reports whether
  KVM is in use.
- **`/dev/net/tun`** — lets QEMU create tap devices for guest NICs. The tap
  interfaces live in the container's own network namespace (we do not use
  `network_mode: host`), so they do not affect the host network stack.

### QEMU config

The wrapper also mounts a `qemu.conf` with:

```text
user = "root"
group = "root"
remember_owner = 0
```

`remember_owner = 0` is required: without it libvirt tries to set the
`trusted.libvirt.security.dac` xattr to remember/restore file ownership, which
needs `CAP_SYS_ADMIN` that a least-privilege container does not have (starting
a VM otherwise fails with "Unable to set XATTR ...").

## Tested versions

The default image is used (built from
[`teran/libvirtd-container`](https://github.com/teran/libvirtd-container)):

```text
ghcr.io/teran/libvirtd-container/libvirtd:v0.1.0
```

## How to use

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/teran/go-docker-testsuite/applications/libvirtd"
)

func main() {
    ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
    defer cancel()

    app, err := libvirtd.New(ctx)
    if err != nil {
        panic(err)
    }
    defer app.Close(ctx)

    // app.Client() exposes the full go-libvirt API.
    ver, err := app.Client().ConnectGetLibVersion()
    if err != nil {
        panic(err)
    }
    fmt.Printf("libvirt version: %d (KVM: %v)\n", ver, app.HasKVM())
}
```

## Running the tests

The example requires a running Docker daemon:

```sh
go test -run Example ./applications/libvirtd/
```

The integration tests (VM boot, network) additionally require a Docker daemon
with a proper cgroup v2 hierarchy and run libvirtd in privileged mode.
