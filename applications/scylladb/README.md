# ScyllaDB

Runs a ScyllaDB container for integration testing and returns a
`scylladb.ScyllaDB` typed client backed by
[`github.com/gocql/gocql`](https://github.com/gocql/gocql). The wrapper starts
a single-node ScyllaDB instance, waits for the CQL server to accept
connections, and exposes helpers to build a cluster config and create/drop
keyspaces.

The client interface provides `ClusterConfig(keyspaceName)`,
`CreateKeyspace(name)`, `DropKeyspace(name)`, and `Close(ctx)`.

## Tested versions

These live as versioned integration tests under
`applications/scylladb/versions/`, one directory per version:

| Version  | Image                                      |
|----------|--------------------------------------------|
| 2025.1.1 | `index.docker.io/scylladb/scylla:2025.1.1` |
| 6.0.0    | `index.docker.io/scylladb/scylla:6.0.0`    |
| 6.1.5    | `index.docker.io/scylladb/scylla:6.1.5`    |
| 6.2.3    | `index.docker.io/scylladb/scylla:6.2.3`    |

## Default image

When you call `scylladb.New(ctx)` without an explicit image, the default
(`images.ScyllaDB`) is used:

```text
index.docker.io/scylladb/scylla:2026.2.0
```

Use `scylladb.NewWithImage(ctx, image)` to select a different version.

## How to use

```go
package main

import (
 "context"
 "time"

 "github.com/teran/go-docker-testsuite/applications/scylladb"
)

func main() {
 ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
 defer cancel()

 app, err := scylladb.New(ctx)
 if err != nil {
  panic(err)
 }
 defer app.Close(ctx)

 if err := app.CreateKeyspace("example_ks"); err != nil {
  panic(err)
 }

 cfg, err := app.ClusterConfig("example_ks")
 if err != nil {
  panic(err)
 }

 session, err := cfg.CreateSession()
 if err != nil {
  panic(err)
 }
 defer session.Close()

 if err := session.Query(
  `CREATE TABLE users (id UUID PRIMARY KEY, name text, email text)`,
 ).Exec(); err != nil {
  panic(err)
 }
}
```

## Running the versioned tests

The versioned tests require a running Docker daemon:

```sh
go test ./applications/scylladb/versions/...
```

You can target a single version, for example:

```sh
go test ./applications/scylladb/versions/6.2.3/
```
