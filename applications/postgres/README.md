# PostgreSQL

Runs a PostgreSQL container for integration testing and returns a
`postgresql.PostgreSQL` typed client backed by
[`github.com/jackc/pgx/v5`](https://pkg.go.dev/github.com/jackc/pgx/v5). The
wrapper starts the container, waits for it to accept connections, and exposes
helpers to build a DSN and create/drop databases.

The client interface provides `DSN(db)` / `MustDSN(db)`, `CreateDB(ctx, db)`,
`DropDB(ctx, db)`, and `Close(ctx)`.

## Tested versions

These live as versioned integration tests under
`applications/postgres/versions/`, one directory per version:

| Version | Image                                    |
|---------|------------------------------------------|
| 10.21   | `index.docker.io/library/postgres:10.21` |
| 11.16   | `index.docker.io/library/postgres:11.16` |
| 12.22   | `index.docker.io/library/postgres:12.22` |
| 13.23   | `index.docker.io/library/postgres:13.23` |
| 14.24   | `index.docker.io/library/postgres:14.24` |
| 15.19   | `index.docker.io/library/postgres:15.19` |
| 16.15   | `index.docker.io/library/postgres:16.15` |
| 17.11   | `index.docker.io/library/postgres:17.11` |
| 18.6    | `index.docker.io/library/postgres:18.6`  |

## Default image

When you call `postgres.New(ctx)` without an explicit image, the default
(`images.Postgres`) is used:

```text
index.docker.io/library/postgres:16.15
```

Use `postgres.NewWithImage(ctx, image)` to select a different version.

## How to use

```go
package main

import (
 "context"
 "time"

 "github.com/jackc/pgx/v5"

 "github.com/teran/go-docker-testsuite/applications/postgres"
)

func main() {
 ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
 defer cancel()

 app, err := postgres.New(ctx)
 if err != nil {
  panic(err)
 }
 defer app.Close(ctx)

 if err := app.CreateDB(ctx, "example_db"); err != nil {
  panic(err)
 }

 conn, err := pgx.Connect(ctx, app.MustDSN("example_db"))
 if err != nil {
  panic(err)
 }
 defer conn.Close(ctx)

 var result int
 if err := conn.QueryRow(ctx, "SELECT 42").Scan(&result); err != nil {
  panic(err)
 }
}
```

## Running the versioned tests

The versioned tests require a running Docker daemon:

```sh
go test ./applications/postgres/versions/...
```

You can target a single version, for example:

```sh
go test ./applications/postgres/versions/18.6/
```
