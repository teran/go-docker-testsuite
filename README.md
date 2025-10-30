# go-docker-testsuite

![Test & Build status](https://github.com/teran/go-docker-testsuite/actions/workflows/verify.yml/badge.svg)
[![Go Report Card](https://goreportcard.com/badge/github.com/teran/go-docker-testsuite)](https://goreportcard.com/report/github.com/teran/go-docker-testsuite)
[![Go Reference](https://pkg.go.dev/badge/github.com/teran/go-docker-testsuite.svg)](https://pkg.go.dev/github.com/teran/go-docker-testsuite)

Library to run any third-party dependency in Docker on any platform Docker supports.
The main purpose is to allow running integration tests against almost any
database or other dependencies running right within docker and accessible via
network.

## Applications

The test suite provides some of applications, i.e. wrappers for particular
docker image, here's the list:

* MinIO
* MySQL/MariaDB/Percona Server
* PostgreSQL
* Redis
* ScyllaDB

Each application could provide its own interface to interact so please refer
to applications package for some examples.

## Example usage

go-docker-testsuite provides its own interface for each application aiming to
make it clean and easy to use them.

Here's an example for MySQL database:

```go
package main

import (
    "context"
    "database/sql"

    _ "github.com/go-sql-driver/mysql"

    "github.com/teran/go-docker-testsuite/v2/applications/mysql"
)

func main() {
    ctx := context.Background()

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

    if err := db.Ping(); err != nil {
        panic(err)
    }

    if _, err := db.ExecContext(ctx, "SELECT 1"); err != nil {
        panic(err)
    }
}

```
