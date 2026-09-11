# go-docker-testsuite — Specification

## Overview

A Go library that launches third-party Docker containers for integration
testing. Tests spin up real instances of databases, message queues, caches,
or object storage — without mocks.

## Architecture

```text
                    ┌─────────────────────┐
                    │   Container (iface)  │
                    ├─────────────────────┤
                    │  Run / Close / Ping  │
                    │  AwaitOutput / URL   │
                    └────────┬────────────┘
                             │
              ┌──────────────┼──────────────┐
              │              │              │
     ┌────────▼─────┐ ┌─────▼──────┐ ┌─────▼──────────┐
     │  Application │ │    Group   │ │  *Application  │
     │ (hooks wrap) │ │ (network)  │ │  (typed wraps) │
     └──────────────┘ └────────────┘ └────────────────┘
```

### Core layer (`package docker`)

| Type | Responsibility |
| ------ | ---------------- |
| `Container` | Interface: `Run`, `Close`, `Ping`, `AwaitOutput`, `GetOutput`, `URL`, `NetworkAttach`, `Name`, `ID`, `Exec` |
| `ExecResult` | Output + exit status of an `Exec` call (`Stdout`, `Stderr`, `ExitCode`); helpers `Error()`, `Combined()` |
| `LifecycleOption` | Configures exec commands that run inside the container during `Run` (startup / after-ready) |
| `WithStartupCommand` | Lifecycle option: command run via exec right after start, before `Run` returns |
| `WithAfterReadyCommand` | Lifecycle option: command run via exec once readiness (a matching log line) is satisfied |
| `WithFiles` | Lifecycle option: copy files into the container filesystem before start (before `WithStartupCommand`) |
| `File` | A file to copy: content bytes, permissions (`Mode`), numeric owner (`Uid`/`Gid`, default `root:root`), and absolute in-container `Destination` |
| `WithHostConfig` | Adapter to combine existing `ContainerOption`s with `LifecycleOption`s in `NewContainerWithLifecycle` |
| `NewContainerWithLifecycle` | `NewContainer` + lifecycle options (existing `NewContainer` unchanged) |
| `TestContainer` | Wraps a `Container` + `*testing.T`; implements the full `Container` interface, registering a `t.Cleanup` on first `Run` and making `Close` idempotent |
| `TestGroup` | Wraps a `Group` + `*testing.T`; registers a `t.Cleanup` on first `Run` and makes `Close` idempotent |
| `BindToT` | Wraps an existing `Container` with a `*testing.T` (`BindToT(t, c) *TestContainer`) |
| `BindGroupToT` | Wraps an existing `Group` with a `*testing.T` (`BindGroupToT(t, g) *TestGroup`) |
| `NewContainerWithT` | `NewContainer` bound to a `*testing.T`, returning a `*TestContainer` |
| `NewContainerWithLifecycleT` | `NewContainerWithLifecycle` bound to a `*testing.T`, returning a `*TestContainer` |
| `NewGroupT` | `NewGroup` bound to a `*testing.T`, returning a `*TestGroup` |
| `RunT` | Fail-fast variant of `Run` on `TestContainer`/`TestGroup` — fails the test via `t.Fatal` instead of returning the error |
| `container` | Concrete impl: Docker API client, image pull + create + start + stop + remove |
| `ContainerOption` | Modifies the Docker `HostConfig` (e.g. `WithPrivileged`, `WithTmpfs`, `WithBinds`, `WithUlimit`, `WithDevices`, `WithCapAdd`, `WithCapDrop`, `WithSecurityOpt`, `WithMemoryLimit`, `WithMemoryReservation`, `WithMemorySwap`, `WithCPUs`, `WithCpusetCpus`, `WithPidsLimit`) |
| `ContainerInfo` | Resolves external port mappings and the Docker host IP |
| `Application` | Wraps `Container` with lifecycle hooks (`BeforeRun`, `AfterRun`, `BeforeClose`, `AfterClose`) |
| `Group` | Isolated internal Docker network; runs multiple `Application`s with DNS resolution |
| `Environment` | Fluent DSL for typed env vars (`StringVar`, `IntVar`, `BoolVar`, etc.) |
| `PortBindings` | DNAT port mapping: random or one-to-one allocation |
| `Matcher` | `func(line string) bool` — substring, exact, or regexp |

#### Resource limits (host/CI protection)

A small, orthogonal set of `ContainerOption`s guards the host and CI runners
from runaway test containers. Each option maps to exactly one `HostConfig`
field, follows the existing `WithX(...) ContainerOption` convention, and takes
raw numeric/string values (no parsing, so it cannot fail):

- `WithMemoryLimit(bytes int64)` — hard RAM limit (`HostConfig.Memory`,
  bytes). `0` = no limit. Enforcing a hard limit may let the kernel OOM-kill
  the application inside the container, which can be undesirable for some
  tests.
- `WithMemoryReservation(bytes int64)` — soft memory limit
  (`HostConfig.MemoryReservation`, bytes). Best-effort; the container may
  exceed it under pressure.
- `WithMemorySwap(bytes int64)` — total memory + swap limit
  (`HostConfig.MemorySwap`). Set equal to the memory limit to disable swap, or
  `-1` for unlimited swap. Unlike `docker run --memory`, it is **not**
  auto-derived from `WithMemoryLimit`; set it explicitly. For the CLI-style
  `2x` behaviour pass `WithMemorySwap(2 * memBytes)`.
- `WithCPUs(count float64)` — CPU usage cap in vCPUs (`HostConfig.NanoCPUs`).
  Values `<= 0` are ignored (no limit). Fractional limits `< 1` rely on the CFS
  quota and can be inaccurate on CI runners with few vCPUs.
- `WithCpusetCpus(cpus string)` — pin to a set of host CPUs
  (`HostConfig.CpusetCpus`, e.g. `"0-2,4"`).
- `WithPidsLimit(limit int64)` — cap on processes/threads
  (`HostConfig.PidsLimit`); protects the runner from fork bombs. `0`/`-1` =
  unlimited.
- `ParseRAMSize(s string) (int64, error)` — helper wrapping
  `github.com/docker/go-units.RAMInBytes` to turn `"512m"`/`"1g"` into bytes.
  Returns an error (never panics) per the `pkg/errors` convention, so string
  sizes can be parsed once and passed to the byte-based options.

Options are independent and order-independent: each writes one field, so a
later option never silently overrides an earlier one. Callers are responsible
for cross-field invariants (e.g. not setting `MemorySwap < Memory`); Docker
rejects those at create time. CFS `CPUQuota`/`CPUPeriod`, `CPUShares`,
`MemorySwappiness` and the Windows-only `CPUCount`/`CPUPercent` are deliberately
left out of the core for now and can be added on demand.

### Exec & lifecycle commands

`Container.Exec(ctx, cmd) (*ExecResult, error)` runs a command inside the
running container via the Docker exec API (`ContainerExecCreate` + `Attach` +
`Inspect`). It captures stdout, stderr, and the exit code. A non-zero exit code
is reported in the result, not as an `Exec` error, so tests can assert on
failure; `ExecResult.Error()` provides a must-succeed check. `Exec` requires a
running container and returns a wrapped error otherwise.

`WithStartupCommand` / `WithAfterReadyCommand` are `LifecycleOption`s executed
inside `container.Run`, so they fire in both standalone and Group flows (Group
delegates to `container.Run`). They are complementary to, not a replacement
for, the Group hooks.

```text
Group.Run (per application):
  1. Hook: BeforeRun
  2. Container.Run:
       a. pull image → create → attach to network
       b. copy files (WithFiles)                      [if set]
       c. start
       d. Exec(WithStartupCommand)                    [if set]
       e. AwaitOutput(ready matcher)                  [if WithAfterReadyCommand set]
       f. Exec(WithAfterReadyCommand)                 [if set]
  3. Hook: AfterRun
```

Readiness for the after-ready command is defined by a log line matching the
given `Matcher` (via `AwaitOutput`); it is NOT `Ping`, which only checks the
Docker daemon. A non-zero exit code from either lifecycle command fails `Run`
(fail-fast on broken init). Lifecycle-command timeouts follow the caller's
`Run` context; a bounded per-command timeout is applied when the context has no
deadline.

### Copying files into the container (`WithFiles`)

`WithFiles(files ...File)` is a `LifecycleOption` that seeds files into the
container filesystem during `Run`, immediately after the container is created
(and the network attached) and **before it starts** — and therefore before
`WithStartupCommand` runs. Because the copy happens before start, both the
image's own entrypoint/CMD and `WithStartupCommand` can consume the files (e.g.
Postgres init scripts dropped into `/docker-entrypoint-initdb.d/`). It works in
both the standalone and Group flows, since Group delegates to `container.Run`.

```go
type File struct {
    Content     io.Reader   // streamed file content; Size bytes must be available
    Size        int64       // exact byte length of Content (required)
    Mode        os.FileMode // permission bits; 0 defaults to 0644
    Destination string      // absolute path inside the container, e.g. "/etc/app.conf"
    Uid         int         // numeric owner uid; 0 (default) = root
    Gid         int         // numeric owner gid; 0 (default) = root
}

// FileFromBytes builds a File from an in-memory byte slice (sets Size to
// len(data)); use File directly with an io.Reader + Size for large files.
func FileFromBytes(destination string, data []byte, mode os.FileMode, uid, gid int) File
```

Files are packed into a single uncompressed tar (standard-library
`archive/tar`) and pushed with the Docker SDK `CopyToContainer` at the
container root (`dstPath = "/"`). Content is **streamed** via `io.CopyN` from
each file's `io.Reader` into the tar (built over an `io.Pipe`), so files larger
than available RAM can be copied without buffering them in memory; `Size` must
equal the exact number of bytes the reader will yield and is written into the
tar header. Parent directories of each `Destination` are auto-created by
emitting explicit directory entries in the tar, so nested paths and
previously-missing directories need no prior setup.

Behavior and edge cases:

- **Empty file list** is a no-op (no copy, no error).
- **`Destination`** must be absolute (start with `/`) and non-empty; paths
  containing `..` are rejected to prevent traversal outside the intended
  target (defence-in-depth).
- **Duplicate destinations** resolve to *last-wins*: `files` are processed in
  order, so a later entry overwrites an earlier one at the same path.
- **Default mode** is `0644`; pass `Mode` explicitly (e.g. `0600`) for files
  that hold secrets.
- **Owner** defaults to `root:root` (`Uid:0`, `Gid:0`). Set `Uid`/`Gid` to
  chown the file inside the container; Docker honours the numeric ids. Only the
  file itself is chowned — the auto-created parent directories remain
  `root:root` (`0755`). Backward compatible: existing calls that omit
  `Uid`/`Gid` behave exactly as before.
- **Errors** during copy (invalid destination, tar/CopyToContainer failure) are
  wrapped with `pkg/errors` and fail `Run` (fail-fast), consistent with the
  other lifecycle steps.
- **Large files** are supported without buffering: pass an `io.Reader` +
  `Size` directly (content is streamed into the tar); use `FileFromBytes` for
  small configuration/seed content. For very large payloads a bind mount
  (`WithBinds`) is still a lighter-weight alternative.

The `Container` interface is **not** extended: file seeding is a
configuration-time concern, expressed as an option like `WithBinds`, rather
than a runtime method. This keeps the interface stable for existing application
packages and mock implementers.

### Testing.T binding

`TestContainer` and `TestGroup` are **decorators** that tie a container or
group's lifecycle to a `*testing.T`. `TestContainer` embeds the full
`Container` interface by delegating every method to the wrapped container, so
it can be passed anywhere a `Container` is expected (including `Application`
and `Group`). `TestGroup` likewise delegates `Run`/`Close` to the wrapped
`Group`.

Binding is purely additive:

- `NewContainerWithT(t, name, image, cmd, env, ports, opts...) *TestContainer`
  mirrors `NewContainer`.
- `NewContainerWithLifecycleT(t, name, image, cmd, env, ports, opts...)
  *TestContainer` mirrors `NewContainerWithLifecycle`.
- `BindToT(t, c Container) *TestContainer` wraps an already-created container.
- `NewGroupT(t, name, apps...) *TestGroup` mirrors `NewGroup`.
- `BindGroupToT(t, g Group) *TestGroup` wraps an already-created group.

The base `Container` interface and `Run(ctx) error` are **unchanged**; binding
uses `*testing.T` (safe for concurrent use) and routes lifecycle events to
`t.Logf` instead of logrus.

**When `t.Cleanup` is registered.** On the **first** `Run` (guarded by a
`sync.Once`), *before* the underlying run starts, a `t.Cleanup` handler is
registered. The handler closes the container/group with a 30-second timeout.
Because it is registered before the run, cleanup covers both the run and the
wait-for-readiness phase, and it fires whether the test succeeds, calls
`t.Fatal`, panics, or skips — in **LIFO order** within that test's context.

**Idempotency.** `TestContainer.Close`, `TestGroup.Close`, and the base
`container.Close` are all idempotent (guarded by a `sync.Once` /
`sync.Mutex`+flag respectively): only the first call performs the work. This
makes overlapping cleanup paths safe — e.g. a `TestContainer`'s `t.Cleanup`
handler racing an explicit wrapper `Close`, or a `Group.Close` also closing an
individually-bound member container — without double-removing anything.

**Fail-fast vs error-return contract.** Two start methods are provided:

- `Run(ctx) error` — returns the error (logging it via `t.Logf`) rather than
  failing the test, preserving compatibility with the `Container` contract,
  applications, and groups.
- `RunT(ctx)` — fail-fast: calls `t.Fatal` on start failure. Use this in
  tests.

**`t.Parallel()` guarantees.** Each binding owns its own lifecycle: cleanups
are registered against the correct per-test `*testing.T` and run in that
test's context, so parallel tests do not interfere with one another's
teardown. Container/network names and host ports are randomized per container,
so concurrent tests do not collide. Bindings themselves are safe for
concurrent use.

### Application layer (`applications/`)

Each sub-package wraps a specific service and returns a typed client:

| Package | Service | Client library |
| --------- | --------- | ---------------- |
| `applications/ceph` | Ceph RGW (S3) | `github.com/aws/aws-sdk-go-v2/service/s3` |
| `applications/k3s` | K3s (Kubernetes) | `k8s.io/client-go` |
| `applications/kafka` | Apache Kafka | `github.com/IBM/sarama` |
| `applications/memcache` | Memcached | `github.com/bradfitz/gomemcache` |
| `applications/minio` | MinIO / Silo (S3) | `github.com/minio/minio-go/v7` |
| `applications/mysql` | MySQL / MariaDB / Percona | `github.com/go-sql-driver/mysql` |
| `applications/opensearch` | OpenSearch | `github.com/opensearch-project/opensearch-go/v4` |
| `applications/postgres` | PostgreSQL | `github.com/jackc/pgx/v5` |
| `applications/rabbitmq` | RabbitMQ | standard library (`net/http`, `encoding/json`) |
| `applications/redis` | Redis | `github.com/go-redis/redis/v8` |
| `applications/scylladb` | ScyllaDB (CQL) | `github.com/gocql/gocql` |
| `applications/vault` | HashiCorp Vault | `github.com/hashicorp/vault-client-go` |
| `applications/libvirtd` | libvirtd (KVM/QEMU) | `github.com/digitalocean/go-libvirt` |

Every application package returns a typed client interface and exposes a
`Close(ctx context.Context) error` method. The rest of the surface differs by
service — there is **no single shared `App` interface**. Database wrappers
share a common DDL contract:

```go
type DB interface {
    Close(ctx context.Context) error
    MustDSN(db string) string
    DSN(db string) (string, error)
    CreateDB(ctx context.Context, name string) error
}
```

Other wrappers expose service-specific accessors instead (e.g. redis `Addr`,
rabbitmq `GetAMQPURL`/`GetManagementURL`, ceph `Endpoint`/`Client`,
k3s `Clientset`/`KubeconfigPath`).

### Image resolution

- `IMAGE_PREFIX` env var prepends a registry mirror to all image references.
- Images with a tag other than `:latest` are cached locally and only pulled
  if missing.
- `:latest` images are compared against the remote registry by manifest
  digest and re-pulled only when the digests differ (best-effort: if the
  remote digest cannot be determined, the image is pulled as before).

### Hooks lifecycle

Hooks are orchestrated by `Group` (and carried by `Application`) around
`container.Run` / `container.Close`. The low-level `container` itself does
**not** invoke hooks.

```text
Group.Run (per application):
  1. Hook: BeforeRun
  2. Container.Run (pull image → create → attach to network → start)
  3. Hook: AfterRun

Group.Close (per application, in reverse order):
  1. Hook: BeforeClose
  2. Container.Close (stop → remove)
  3. Hook: AfterClose
  4. Remove the internal network
```

## Dependencies

- **Go 1.26+** — required by `go.mod` directive.
- **Docker daemon** — local or remote (`DOCKER_HOST` et al.).
- Uses the official Docker SDK (`github.com/docker/docker`) — no shell-outs
  to the `docker` CLI.

## Conventions

- **No mocks in tests** — real Docker containers only (skippable without Docker).
- **Testable Examples** (`Example*` functions) in every application package.
- **Versioned integration tests** live under `applications/*/versions/`.
- **Error wrapping** uses `github.com/pkg/errors` consistently.
- **Logging** uses `github.com/sirupsen/logrus` — trace-level for internals.
- **T-bound constructors are additive** — the base `Container` interface and
  `Run(ctx)` are unchanged; binding uses `*testing.T` (safe for concurrent use)
  and routes lifecycle events to `t.Logf`.

## CI

- **markdownlint** — all `.md` files must conform to `.markdownlint.json` rules.
- **golangci-lint** — mandatory before every commit.
- **govulncheck** — fails CI only on fixable vulnerabilities; findings with no
  available fix are reported for visibility but do not redden CI.
- **Tests** — automatically discovered and split into parallel CI groups by a
  Go program (`go run ./tools/cmd/split_test_groups`). A dedicated `discover`
  job runs it and feeds a dynamic matrix to the `tests` job. New test packages
  are picked up automatically without editing workflow files.
- **Integration tests** — require a running Docker daemon; run on CI runners
  (`ubuntu-latest`) with full container orchestration.

## Security

- Application wrappers validate database/keyspace names to prevent SQL/CQL
  injection through DDL identifiers.
- See [SECURITY.md](./SECURITY.md) for the vulnerability reporting policy.
