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
- **Matchers** — await container logs with substring, exact,
  or regexp matchers before proceeding
- **Environment builder** — fluent DSL to declare typed environment variables
- **Port bindings** — DNAT port mapping with random or one-to-one port allocation
- **IMAGE_PREFIX** — optional `IMAGE_PREFIX` env var to route images through a proxy/mirror

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

| Application                                                | Package                                            | Description                      |
| ---------------------------------------------------------- | -------------------------------------------------- | -------------------------------- |
| [Kafka](https://kafka.apache.org/)                         | [`applications/kafka`](./applications/kafka)       | Apache Kafka with Sarama client  |
| [Memcache](https://memcached.org/)                         | [`applications/memcache`](./applications/memcache) | Memcached with gomemcache client |
| [MinIO](https://min.io/)                                   | [`applications/minio`](./applications/minio)       | S3-compatible object storage     |
| [MySQL / MariaDB / Percona Server](https://www.mysql.com/) | [`applications/mysql`](./applications/mysql)       | MySQL-compatible databases       |
| [PostgreSQL](https://www.postgresql.org/)                  | [`applications/postgres`](./applications/postgres) | PostgreSQL with pgx client       |
| [Redis](https://redis.io/)                                 | [`applications/redis`](./applications/redis)       | Redis with go-redis client       |
| [ScyllaDB](https://www.scylladb.com/)                      | [`applications/scylladb`](./applications/scylladb) | ScyllaDB with gocql client       |
| [Vault](https://www.vaultproject.io/)                      | [`applications/vault`](./applications/vault)       | HashiCorp Vault                  |
| —                                                          | `applications/*/versions/`                         | Per-version integration tests    |

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

### Lifecycle hooks

Every container supports hooks at four stages:

```go
docker.HookTypeBeforeRun   // before container starts
docker.HookTypeAfterRun    // after container starts
docker.HookTypeBeforeClose // before container stops
docker.HookTypeAfterClose  // after container stops
```

Pass hooks via `docker.NewApplication(container, hook1, hook2, ...)`.

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
