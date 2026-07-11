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
| `Container` | Interface: `Run`, `Close`, `Ping`, `AwaitOutput`, `GetOutput`, `URL`, `NetworkAttach`, `Name` |
| `container` | Concrete impl: Docker API client, image pull + create + start + stop + remove |
| `Application` | Wraps `Container` with lifecycle hooks (`BeforeRun`, `AfterRun`, `BeforeClose`, `AfterClose`) |
| `Group` | Isolated internal Docker network; runs multiple `Application`s with DNS resolution |
| `Environment` | Fluent DSL for typed env vars (`StringVar`, `IntVar`, `BoolVar`, etc.) |
| `PortBindings` | DNAT port mapping: random or one-to-one allocation |
| `Matcher` | `func(line string) bool` — substring, exact, or regexp |

### Application layer (`applications/`)

Each sub-package wraps a specific service and returns a typed client:

| Package | Service | Client library |
| --------- | --------- | ---------------- |
| `applications/kafka` | Apache Kafka | `github.com/IBM/sarama` |
| `applications/memcache` | Memcached | `github.com/bradfitz/gomemcache` |
| `applications/minio` | MinIO (S3) | `github.com/minio/minio-go/v7` |
| `applications/mysql` | MySQL / MariaDB / Percona | `github.com/go-sql-driver/mysql` |
| `applications/postgres` | PostgreSQL | `github.com/jackc/pgx/v4` |
| `applications/redis` | Redis | `github.com/go-redis/redis/v8` |
| `applications/scylladb` | ScyllaDB (CQL) | `github.com/gocql/gocql` |
| `applications/vault` | HashiCorp Vault | `github.com/hashicorp/vault-client-go` |

Every application package follows the same contract:

```go
type App interface {
    Close(ctx context.Context) error
    MustDSN(db string) string
    DSN(db string) (string, error)
    CreateDB(ctx context.Context, name string) error
}
```

### Image resolution

- `IMAGE_PREFIX` env var prepends a registry mirror to all image references.
- Images with a tag other than `:latest` are cached locally and only pulled
  if missing; `:latest` is always re-pulled.

### Hooks lifecycle

```text
Container.Run:
  1. Pull image
  2. Create container
  3. Attach to network (if Group)
  4. Hook: BeforeRun
  5. ContainerStart
  6. Hook: AfterRun

Container.Close:
  1. Hook: BeforeClose
  2. ContainerStop (+ ContainerRemove)
  3. Hook: AfterClose
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
- **Test assertions** use `github.com/stretchr/testify` throughout.

## CI

- **markdownlint** — all `.md` files must conform to `.markdownlint.json` rules.
- **golangci-lint** — mandatory before every commit.
- **Tests** — split into parallel CI jobs via matrix strategy to minimise wall-clock
  time. Each job runs with `-race` and `-timeout` appropriate to its group.
  - **core** — root package (`.`) and `./internal/...`; includes the coverage gate.
  - **postgres-1** / **postgres-2** — PostgreSQL and its version tests.
  - **scylladb** — ScyllaDB and its version tests.
  - **datastores** — MySQL, Redis, Memcache, MinIO, Vault.
  - **messaging** — Kafka and RabbitMQ.
- **Coverage gate** — overall statement coverage of the core package and internal
  helpers must be **≥85%**. Measured by `go test -coverprofile` on the `core` group
  and checked via `go tool cover` in CI.
- **Integration tests** — require a running Docker daemon; run on CI runners
  (`ubuntu-latest`) with full container orchestration.

## Security

- Application wrappers validate database/keyspace names to prevent SQL/CQL
  injection through DDL identifiers.
- See [SECURITY.md](./SECURITY.md) for the vulnerability reporting policy.
