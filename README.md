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

| Application                                                | Package                                                | Description                          |
|------------------------------------------------------------|--------------------------------------------------------|--------------------------------------|
| [Ceph (RGW)](https://ceph.io/)                             | [`applications/ceph`](./applications/ceph)             | Ceph RGW (S3) with AWS SDK v2 client |
| [Kafka](https://kafka.apache.org/)                         | [`applications/kafka`](./applications/kafka)           | Apache Kafka with Sarama client      |
| [Memcache](https://memcached.org/)                         | [`applications/memcache`](./applications/memcache)     | Memcached with gomemcache client     |
| [MinIO](https://min.io/)                                   | [`applications/minio`](./applications/minio)           | S3-compatible object storage         |
| [MySQL / MariaDB / Percona Server](https://www.mysql.com/) | [`applications/mysql`](./applications/mysql)           | MySQL-compatible databases           |
| [OpenSearch](https://opensearch.org/)                      | [`applications/opensearch`](./applications/opensearch) | OpenSearch with opensearch-go client |
| [PostgreSQL](https://www.postgresql.org/)                  | [`applications/postgres`](./applications/postgres)     | PostgreSQL with pgx client           |
| [Redis](https://redis.io/)                                 | [`applications/redis`](./applications/redis)           | Redis with go-redis client           |
| [ScyllaDB](https://www.scylladb.com/)                      | [`applications/scylladb`](./applications/scylladb)     | ScyllaDB with gocql client           |
| [Vault](https://www.vaultproject.io/)                      | [`applications/vault`](./applications/vault)           | HashiCorp Vault                      |
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

## Tested versions

Each application wrapper is exercised against the exact container image
versions listed below via per-version integration tests in
`applications/<app>/versions/`. Applications without a `versions/`
directory are validated against their default image only.

| Application                           | Tested versions                                                                       |
|---------------------------------------|---------------------------------------------------------------------------------------|
| [Ceph (RGW)](https://ceph.io/)        | 19.2.0, 19.2.1, 19.2.2, 19.2.3, 19.2.4, 19.2.5, 19.2.6, 20.2.0, 20.2.1, 20.2.2, 20.2.3, 20.2.4 |
| [Kafka](https://kafka.apache.org/)    | 4.1.2, 4.2.1, 4.3.1                                                                   |
| [K3s](https://k3s.io/)                | v1.31.14, v1.32.13, v1.33.13, v1.34.9, v1.35.6, v1.36.2                               |
| [MySQL / MariaDB / Percona](https://www.mysql.com/) | MariaDB: 11.4.2, 12.0.2 · MySQL: 8.0.4, 9.5.0 · Percona: 8.0.36-28   |
| [OpenSearch](https://opensearch.org/) | 2.12.0, 2.17.1, 2.19.6                                                                |
| [PostgreSQL](https://www.postgresql.org/) | 10.21, 11.16, 12.22, 13.23, 14.24, 15.19, 16.15, 17.11, 18.6                       |
| [RabbitMQ](https://www.rabbitmq.com/) | 3.13, 4.0                                                                            |
| [Redis](https://redis.io/)            | 6.2.14, 7.0.15, 7.2.5                                                                 |
| [ScyllaDB](https://www.scylladb.com/) | 2025.1.1, 6.0.0, 6.1.5, 6.2.3                                                         |
| [Memcache](https://memcached.org/)    | default image only                                                                    |
| [MinIO](https://min.io/)              | default image only                                                                    |
| [Vault](https://www.vaultproject.io/) | default image only                                                                    |

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
