# go-docker-testsuite

[![Test & Build](https://github.com/teran/go-docker-testsuite/actions/workflows/verify.yml/badge.svg)](https://github.com/teran/go-docker-testsuite/actions/workflows/verify.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/teran/go-docker-testsuite.svg)](https://pkg.go.dev/github.com/teran/go-docker-testsuite)
[![Go Version](https://img.shields.io/github/go-mod/go-version/teran/go-docker-testsuite)](https://github.com/teran/go-docker-testsuite)
[![Last Commit](https://img.shields.io/github/last-commit/teran/go-docker-testsuite)](https://github.com/teran/go-docker-testsuite/commits/master)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](https://github.com/teran/go-docker-testsuite/pulls)

**Go library to run third-party dependencies in Docker containers**
for integration testing. Spin up any Docker image — databases, queues,
caches, or object storage — and connect to them via network from your
Go tests.

---

## Features

- **Container** — low-level wrapper to create, run, await output, and
  clean up any Docker image
- **Group** — run multiple containers in an isolated Docker network
  with IP-level connectivity
- **Applications** — ready-to-use wrappers for popular services
  (MySQL, PostgreSQL, Redis, Kafka, etc.)
- **Hooks** — lifecycle callbacks
  (BeforeRun, AfterRun, BeforeClose, AfterClose) per container
- **Exec** — run commands inside a running container and capture
  stdout, stderr, and the exit code
- **Lifecycle commands** — run startup / after-ready commands via exec
  (`WithStartupCommand`, `WithAfterReadyCommand`)
- **Copy files into containers** — seed files before start with `WithFiles`
  (`FileFromBytes` for small content, or stream large files via `io.Reader` + `Size`)
- **Resource limits** — cap CPU/memory/pids to protect the host and CI
  (`WithMemoryLimit`, `WithCPUs`, `WithPidsLimit`, ...)
- **Matchers** — await container logs with substring, exact,
  or regexp matchers before proceeding
- **Environment builder** — fluent DSL to declare typed environment variables
- **Port bindings** — DNAT port mapping with random or one-to-one port allocation
- **IMAGE_PREFIX** — optional `IMAGE_PREFIX` env var to route images through a proxy/mirror
- **`*testing.T` binding** — bind a container/group to a test for automatic
  teardown (`t.Cleanup`) and `t.Logf` lifecycle logging, with safe `t.Parallel()`

## Requirements

- Go 1.26+ (uses `go.1.26.0` directive in `go.mod`)
- A running Docker daemon (also works with remote Docker hosts
  via `DOCKER_HOST`, etc.)

## Installation

```sh
go get github.com/teran/go-docker-testsuite
```

## Applications

The test suite provides ready-to-use wrappers (each returns a typed
client interface and handles startup, health checks, and cleanup).
Here's the full list:

| Application                                                | Package                                                | Description                          |
|------------------------------------------------------------|--------------------------------------------------------|--------------------------------------|
| [Ceph (RGW)](https://ceph.io/)                             | [`applications/ceph`](./applications/ceph)             | Ceph RGW (S3) with AWS SDK v2 client |
| [Forgejo](https://forgejo.org/)                            | [`applications/forgejo`](./applications/forgejo)       | Forgejo (git hosting) with SQLite    |
| [K3s](https://k3s.io/)                                     | [`applications/k3s`](./applications/k3s)               | K3s (Kubernetes) with client-go      |
| [Kafka](https://kafka.apache.org/)                         | [`applications/kafka`](./applications/kafka)           | Apache Kafka with Sarama client      |
| [Memcache](https://memcached.org/)                         | [`applications/memcache`](./applications/memcache)     | Memcached with gomemcache client     |
| [MinIO (Silo fork)](https://silo.pgsty.com/)               | [`applications/minio`](./applications/minio)           | S3-compatible object storage         |
| [MySQL / MariaDB / Percona Server](https://www.mysql.com/) | [`applications/mysql`](./applications/mysql)           | MySQL-compatible databases           |
| [Nginx](https://nginx.org/)                                | [`applications/nginx`](./applications/nginx)           | nginx reverse proxy / web server     |
| [OpenSearch](https://opensearch.org/)                      | [`applications/opensearch`](./applications/opensearch) | OpenSearch with opensearch-go client |
| [PostgreSQL](https://www.postgresql.org/)                  | [`applications/postgres`](./applications/postgres)     | PostgreSQL with pgx client           |
| [RabbitMQ](https://www.rabbitmq.com/)                      | [`applications/rabbitmq`](./applications/rabbitmq)     | RabbitMQ (AMQP + Management API)     |
| [Redis](https://redis.io/)                                 | [`applications/redis`](./applications/redis)           | Redis with go-redis client           |
| [ScyllaDB](https://www.scylladb.com/)                      | [`applications/scylladb`](./applications/scylladb)     | ScyllaDB with gocql client           |
| [Vault](https://www.vaultproject.io/)                      | [`applications/vault`](./applications/vault)           | HashiCorp Vault                      |
| [Libvirtd](https://libvirt.org/)                           | [`applications/libvirtd`](./applications/libvirtd)     | KVM/QEMU virtualization manager      |
| —                                                          | `applications/*/versions/`                             | Per-version integration tests        |

> **Ceph image:** the Ceph wrapper runs the multi-arch demo image
> `ghcr.io/teran/ceph-container/ceph:v<version>` (built for the squid and
> tentacle release trains). It is built and published independently from
> [github.com/teran/ceph-container](https://github.com/teran/ceph-container).
> Use a specific version (e.g. `ceph.NewWithImage(ctx,
> "ghcr.io/teran/ceph-container/ceph:v20.2.4")`) when the default
> (`images.Ceph`) isn't the one you need.

Many application packages include [testable Examples](https://go.dev/blog/examples)
(`Example*` functions in `*_test.go` files) that demonstrate real
usage. They are displayed on [pkg.go.dev](https://pkg.go.dev/github.com/teran/go-docker-testsuite)
and can be verified locally:

```sh
# Run all examples (requires a running Docker daemon):
go test -run Example ./applications/... .

# Run a specific example:
go test -run "^Example$" ./applications/mysql/
```

## Usage

### Quick start — MySQL

```go
package main

import (
    "context"
    "database/sql"
    "time"

    _ "github.com/go-sql-driver/mysql"

    "github.com/teran/go-docker-testsuite/applications/mysql"
)

func main() {
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
    defer cancel()

    app, err := mysql.New(ctx, "index.docker.io/library/mysql:8.0.4")
    if err != nil {
        panic(err)
    }
    defer app.Close(ctx)

    if err := app.CreateDB(ctx, "important_database"); err != nil {
        panic(err)
    }

    db, err := sql.Open("mysql", app.MustDSN("important_database"))
    if err != nil {
        panic(err)
    }
    defer db.Close()

    if _, err := db.ExecContext(ctx, "SELECT 1"); err != nil {
        panic(err)
    }
}
```

### Multi-container group

Use `docker.Group` to run several containers in an isolated Docker network
with internal DNS resolution:

```go
package main

import (
    "context"
    "time"

    "github.com/teran/go-docker-testsuite"
)

func main() {
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
    defer cancel()

    app := docker.NewApplication(
        c,
        docker.HookFunc(func(ctx context.Context, ht docker.HookType, c docker.Container) error {
            // e.g. wait for readiness before moving on
            return c.AwaitOutput(ctx, docker.NewSubstringMatcher("ready"))
        }),
    )

    g, err := docker.NewGroup("my-services", app1, app2)
    if err != nil {
        panic(err)
    }

    if err := g.Run(ctx); err != nil {
        panic(err)
    }
    defer g.Close(ctx)
}
```

### `*testing.T` binding & `t.Parallel()`

Bind a container (or group) to a `*testing.T` so its lifecycle follows the
test automatically — no manual `defer c.Close`. `TestContainer` wraps a
`Container` and registers a `t.Cleanup` handler on the first `Run`, so the
container is always stopped and removed when the test finishes, whether it
succeeds, calls `t.Fatal`, panics, or skips. Cleanup handlers run in LIFO
order within the test's context. `TestContainer` implements the full
`Container` interface, so it can be used anywhere a `Container` is expected.

```go
func TestRedis(t *testing.T) {
    t.Parallel()

    c, err := docker.NewContainerWithT(
        t,
        "redis",
        images.Redis,
        nil,
        docker.NewEnvironment(),
        docker.NewPortBindings().PortDNAT(docker.ProtoTCP, 6379),
    )
    if err != nil {
        t.Fatal(err)
    }

    c.RunT(ctx) // fail-fast; registers t.Cleanup

    // ... use container ... // no manual defer; cleanup on success/fatal/panic/skip
}
```

Two ways to start a bound container:

- `Run(ctx) error` — returns the error (and logs it via `t.Logf`) rather than
  failing the test. This keeps `TestContainer` compatible with applications,
  groups, and existing code that expects the `Container` contract.
- `RunT(ctx)` — fail-fast: if the container fails to start, the test is
  failed immediately with `t.Fatal`. Prefer this in tests.

To wrap an existing container or group (created with the base
`NewContainer`/`NewContainerWithLifecycle`/`NewGroup` constructors) use
`BindToT(t, c)` or `BindGroupToT(t, g)`:

```go
c, err := docker.NewContainer("my-service", "busybox:latest", nil, nil, nil)
if err != nil {
    t.Fatal(err)
}

docker.BindToT(t, c) // lifecycle now tied to the test
_ = c.Run(ctx)
```

Application packages expose T-bound constructors that create the container
*and* start it, all tied to the test: `applications/postgres`
(`NewWithT(t, ctx)` — uses the default `images.Postgres`, or
`NewWithImageT(t, ctx, image)`), `applications/redis` and `applications/mysql`
(`NewWithT(t, ctx, image)`). PostgreSQL is bound first because it is the most
commonly used; the other application packages will follow.

**Safe `t.Parallel()`.** Each binding owns its own lifecycle: cleanups are
registered against the correct per-test `*testing.T` and run in that test's
context, so parallel tests do not interfere with one another's teardown. To
avoid collisions, container/network names and host ports are randomized per
container, so concurrent tests do not clash. Keep the parallel fan-out modest
and combine `t.Parallel()` with resource limits (`WithMemoryLimit`,
`WithCPUs`, `WithPidsLimit`) to protect the host and CI runners.

### Lifecycle hooks

Every container supports hooks at four stages:

```go
docker.HookTypeBeforeRun   // before container starts
docker.HookTypeAfterRun    // after container starts
docker.HookTypeBeforeClose // before container stops
docker.HookTypeAfterClose  // after container stops
```

Pass hooks via `docker.NewApplication(container, hook1, hook2, ...)`.

### Exec and lifecycle commands

Run a command inside a running container with `Container.Exec` and inspect
its output and exit code:

```go
res, err := c.Exec(ctx, []string{"echo", "hello"})
if err != nil {
    // infrastructure failure (create/attach/inspect or ctx cancelled)
    panic(err)
}

if err := res.Error(); err != nil {
    // non-zero exit code (res.ExitCode, res.Stderr available for diagnostics)
    panic(err)
}

fmt.Printf("exit code: %d\n", res.ExitCode)
fmt.Printf("stdout: %s", res.Stdout)
```

To run initialization commands automatically, use `NewContainerWithLifecycle`
together with `WithStartupCommand` (executed right after the container starts)
and `WithAfterReadyCommand` (executed once a readiness log line appears):

```go
c, err := docker.NewContainerWithLifecycle(
    "my-service",
    "busybox:latest",
    []string{"sh", "-c", "echo READY; sleep 300"},
    nil,
    nil,
    docker.WithStartupCommand("sh", "-c", "echo boot > /tmp/startup.txt"),
    docker.WithAfterReadyCommand(
        docker.NewSubstringMatcher("READY"),
        "sh", "-c", "echo seeded > /tmp/seeded.txt",
    ),
)
if err != nil {
    panic(err)
}
defer c.Close(ctx)

if err := c.Run(ctx); err != nil {
    panic(err)
}
```

For host/CI protection, cap resource usage with the `With*` options (see
[SPEC.md](./SPEC.md) → "Resource limits"):

```go
c, err := docker.NewContainer(
    "my-service",
    "busybox:latest",
    []string{"sleep", "300"},
    nil,
    nil,
    docker.WithMemoryLimit(128*1024*1024), // 128 MiB
    docker.WithCPUs(0.5),                  // half a vCPU
    docker.WithPidsLimit(256),
)
```

### Copying files into the container (WithFiles)

Seed files into the container filesystem before it starts with `WithFiles`.
Files are packed into a tar and pushed via the Docker SDK `CopyToContainer`
before `WithStartupCommand` runs, so both the image entrypoint and startup
commands can consume them. `WithFiles` is a `LifecycleOption`, so it is used
through `NewContainerWithLifecycle`.

For small in-memory config, use the `FileFromBytes` helper:

```go
c, err := docker.NewContainerWithLifecycle(
    "my-service",
    "busybox:latest",
    []string{"sleep", "300"},
    nil,
    nil,
    docker.WithFiles(
        docker.FileFromBytes("/etc/app.conf", []byte("key=value\n"), 0600, 0, 0),
    ),
)
if err != nil {
    panic(err)
}
defer c.Close(ctx)

if err := c.Run(ctx); err != nil {
    panic(err)
}
```

For files larger than available RAM, pass an `io.Reader` and its exact size —
the content is streamed into the tar without buffering in memory:

```go
f, err := os.Open("/data/big.bin")
if err != nil {
    panic(err)
}
defer f.Close()

st, err := f.Stat()
if err != nil {
    panic(err)
}

c, err := docker.NewContainerWithLifecycle(
    "my-service",
    "busybox:latest",
    []string{"sleep", "300"},
    nil,
    nil,
    docker.WithFiles(docker.File{
        Content:     f,
        Size:        st.Size(),
        Mode:        0644,
        Destination: "/data/big.bin",
        // Uid/Gid default to 0 = root:root; set them to chown the file.
    }),
)
```

### Image prefix / proxy

Set the `IMAGE_PREFIX` environment variable to prepend a registry mirror
to all image references:

```sh
# Use a local mirror instead of Docker Hub
export IMAGE_PREFIX=registry-mirror.example.com
```

## Examples

Each application package includes testable examples. Run them with:

```sh
# Run all examples (needs Docker):
go test -run Example ./applications/... .
```

## Project docs

- [SPEC.md](./SPEC.md) — Architecture and design specification
- [AGENTS.md](./AGENTS.md) — Agent instructions for AI-assisted development
- [CONTRIBUTING.md](./CONTRIBUTING.md) — How to contribute
- [CODE_OF_CONDUCT.md](./CODE_OF_CONDUCT.md) — Community guidelines
- [SECURITY.md](./SECURITY.md) — Security policy and vulnerability reporting

## License

This project is licensed under the [Apache License, Version 2.0](LICENSE).
