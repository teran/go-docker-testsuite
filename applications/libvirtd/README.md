# Libvirtd

Runs a [libvirtd](https://libvirt.org/) container (KVM/QEMU virtualization
manager) for integration testing and returns a connected
[`go-libvirt`](https://github.com/digitalocean/go-libvirt) client. The client
exposes the full libvirt API — virtual machine (domain) lifecycle, storage
pools and volumes, virtual networks, snapshots, and more.

The client interface provides `Client()`, `SocketPath()`, `HasKVM()`, and
`Close(ctx)`.

## Privileges and requirements

By default the container runs in a **least-privilege** configuration: all
capabilities are dropped and only the minimal set QEMU/libvirtd need is added
back (networking, storage, process/scheduling), with Docker's default seccomp
profile relaxed (`seccomp=unconfined`) so QEMU can start. For workloads that
need more (e.g. `SYS_ADMIN` for mount/loop-backed storage pools or LVM), pass
`libvirtd.WithPrivileged()` to run in full Docker privileged mode.

`/dev/kvm` and `/dev/net/tun` are passed through **only if they exist on the
host**. When `/dev/kvm` is absent, QEMU falls back to software (TCG)
emulation — slower but functional. `HasKVM()` reports whether KVM
acceleration is in use.

## Tested versions

The default image is used (versioned, actively maintained):

```text
ghcr.io/vexxhost/libvirtd:2025.2
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
