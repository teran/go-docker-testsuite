# Forgejo

Runs a Forgejo container backed by SQLite for integration testing and returns a
`forgejo.Forgejo` typed handle. The wrapper starts the container, waits for the
web server to come up, and programmatically passes the initial setup/onboarding
screen by creating the admin account — so the instance is immediately usable
without manual configuration.

The client interface provides `URL()` / `MustURL()` (the web UI/HTTP endpoint),
`AdminUsername()` / `AdminPassword()` (the credentials of the admin user created
during setup), and `Close(ctx)`.

No Forgejo/Gitea SDK dependency is embedded: this wrapper is deliberately
stdlib-only, and the caller brings their own HTTP/API client (e.g. to create
repositories, manage users, etc.).

## Tested versions

These live as versioned integration tests under
`applications/forgejo/versions/`, one directory per version:

| Version | Image                                |
|---------|--------------------------------------|
| 16.0.4  | `codeberg.org/forgejo/forgejo:16.0.4` |

## Default image

There is no default image. `forgejo.New(ctx, image)` **requires an explicit
image** reference, for example:

```text
codeberg.org/forgejo/forgejo:16
```

## How to use

```go
package main

import (
 "context"
 "net/http"
 "time"

 "github.com/teran/go-docker-testsuite/applications/forgejo"
)

func main() {
 ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
 defer cancel()

 app, err := forgejo.New(ctx, "codeberg.org/forgejo/forgejo:16")
 if err != nil {
  panic(err)
 }
 defer app.Close(ctx)

 resp, err := http.Get(app.MustURL())
 if err != nil {
  panic(err)
 }
 defer resp.Body.Close()
 _ = resp
}
```

## Running the versioned tests

The versioned tests require a running Docker daemon:

```sh
go test ./applications/forgejo/versions/...
```

You can target a single version, for example:

```sh
go test ./applications/forgejo/versions/16.0.4/
```
