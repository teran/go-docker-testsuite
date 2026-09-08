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
| `Container` | Interface: `Run`, `Close`, `Ping`, `AwaitOutput`, `GetOutput`, `URL`, `NetworkAttach`, `Name`, `ID` |
| `container` | Concrete impl: Docker API client, image pull + create + start + stop + remove |
| `ContainerOption` | Modifies the Docker `HostConfig` (e.g. `WithPrivileged`, `WithTmpfs`, `WithBinds`, `WithUlimit`, `WithDevices`, `WithCapAdd`, `WithCapDrop`, `WithSecurityOpt`) |
| `ContainerInfo` | Resolves external port mappings and the Docker host IP |
| `Application` | Wraps `Container` with lifecycle hooks (`BeforeRun`, `AfterRun`, `BeforeClose`, `AfterClose`) |
| `Group` | Isolated internal Docker network; runs multiple `Application`s with DNS resolution |
| `Environment` | Fluent DSL for typed env vars (`StringVar`, `IntVar`, `BoolVar`, etc.) |
| `PortBindings` | DNAT port mapping: random or one-to-one allocation |
| `Matcher` | `func(line string) bool` — substring, exact, or regexp |

### Application layer (`applications/`)

Each sub-package wraps a specific service and returns a typed client:

| Package | Service | Client library |
| --------- | --------- | ---------------- |
| `applications/ceph` | Ceph RGW (S3) | `github.com/aws/aws-sdk-go-v2/service/s3` |
| `applications/k3s` | K3s (Kubernetes) | `k8s.io/client-go` |
| `applications/kafka` | Apache Kafka | `github.com/IBM/sarama` |
| `applications/memcache` | Memcached | `github.com/bradfitz/gomemcache` |
| `applications/minio` | MinIO (S3) | `github.com/minio/minio-go/v7` |
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
