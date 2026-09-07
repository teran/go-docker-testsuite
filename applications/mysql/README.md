# MySQL / MariaDB / Percona Server

Runs a MySQL-compatible database container for integration testing and returns
a `mysql.MySQL` typed client. The wrapper works with **MySQL**, **MariaDB**,
and **Percona Server** images: it starts the container, waits for the server to
accept connections, and exposes helpers to build a DSN and create/drop
databases over `database/sql` (using
[`github.com/go-sql-driver/mysql`](https://github.com/go-sql-driver/mysql)).

The client interface provides `DSN(name)` / `MustDSN(name)`,
`CreateDB(ctx, name)`, `DropDB(ctx, name)`, and `Close(ctx)`.

## Tested versions

These live as versioned integration tests under
`applications/mysql/versions/`, grouped by image family:

| Version | Image |
| ------------- | ------- |
| mysql 8.0.4 | `index.docker.io/library/mysql:8.0.4` |
| mysql 9.5.0 | `index.docker.io/library/mysql:9.5.0` |
| mariadb 11.4.2 | `index.docker.io/library/mariadb:11.4.2` |
| mariadb 12.0.2 | `index.docker.io/library/mariadb:12.0.2` |
| percona 8.0.36-28 | `index.docker.io/library/percona:8.0.36-28` |

## Default image

There is no default image. `mysql.New(ctx, image)` **requires an explicit image**
reference — pick any MySQL, MariaDB, or Percona Server image, for example:

```text
index.docker.io/library/mysql:8.0.4
```

## How to use

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

## Running the versioned tests

The versioned tests require a running Docker daemon:

```sh
go test ./applications/mysql/versions/...
```

You can target a single family or version, for example:

```sh
go test ./applications/mysql/versions/mysql/8.0.4/
go test ./applications/mysql/versions/mariadb/
```
