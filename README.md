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

## Examples

Each application package includes [testable Examples](https://go.dev/blog/examples)
(`Example*` functions in `*_test.go` files) that demonstrate real usage.
They are displayed on [pkg.go.dev](https://pkg.go.dev/github.com/teran/go-docker-testsuite)
and can be validated locally (requires a running Docker daemon):

```sh
# Run all examples (including those that need Docker):
go test -run Example ./applications/... .

# Run a specific example:
go test -run "^Example$" ./applications/mysql/
```

See `Example()` functions in each application package for ready-to-use code.

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
