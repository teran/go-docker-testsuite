# Paperless-ngx

Runs a [Paperless-ngx](https://docs.paperless-ngx.com/) (document management
system) instance for integration testing and returns a `paperlessngx.PaperlessNGX`
typed handle. Like NetBox, Paperless-ngx cannot run on SQLite in this wrapper —
it requires an external PostgreSQL database and a Valkey/Redis message broker —
so this is a **multi-container `docker.Group`-based** wrapper. It spins up three
containers on a shared internal network where the siblings resolve each other by
container name:

- `paperless-postgres` (PostgreSQL, internal only)
- `paperless-valkey` (Valkey / Redis-protocol broker, internal only)
- `paperless` (the Paperless-ngx web server, port `8000` DNAT'd to the host)

Two additional containers are spun up only when their corresponding option is
set. They are used exclusively at document-consumption time — the web server
boots regardless of them:

- `paperless-gotenberg` (Gotenberg, office → PDF conversion, internal only)
- `paperless-tika` (Apache Tika, office text extraction, internal only)

Only the Paperless-ngx web container's `:8000` is published; PostgreSQL, Valkey,
Gotenberg and Tika stay internal and are never exposed to the host.

The client interface provides `URL()` / `MustURL()` (the web UI / REST API root,
resolved to its own random host port), `Username()` / `Password()` (the admin
credentials created during setup, used as HTTP Basic auth), and `Close(ctx)`.

No Paperless-ngx SDK dependency is embedded: this wrapper is deliberately
stdlib-only, and the caller brings their own HTTP client (e.g. `net/http`) — the
wrapper only hands back the URL and credentials.

## Options

Constructors accept optional `Option`s (variadic):

- `paperlessngx.WithSecretKey("...")` — sets `PAPERLESS_SECRET_KEY`. Defaults to
  a fixed, deterministic and deliberately insecure test-only value. The server
  refuses to start without a secret key, so a default is always provided.
- `paperlessngx.WithDBPassword("...")` — sets the password for the backing
  PostgreSQL (`POSTGRES_PASSWORD` / `PAPERLESS_DBPASS`). Defaults to a fixed
  test-only constant.
- `paperlessngx.WithAdmin("user", "pass", "user@example.com")` — overrides the
  admin account created during setup (`PAPERLESS_ADMIN_USER` /
  `PAPERLESS_ADMIN_PASSWORD` / `PAPERLESS_ADMIN_MAIL`). Defaults to a
  deterministic test account (`admin`).
- `paperlessngx.WithAllowedHosts("a.example", "b.example")` — sets
  `PAPERLESS_ALLOWED_HOSTS` (comma-joined into a single env var). When unset,
  the Paperless-ngx default (`*`) is left in place so the instance answers on
  any host.
- `paperlessngx.WithGotenberg()` — enables the optional Gotenberg container and
  wires `PAPERLESS_GOTENBERG_ENDPOINT`, so Paperless-ngx can convert office
  documents to PDF at consumption time.
- `paperlessngx.WithTika()` — enables the optional Apache Tika container and
  wires `PAPERLESS_TIKA_ENABLED` / `PAPERLESS_TIKA_ENDPOINT`, so Paperless-ngx
  can extract text from office documents at consumption time.

All defaults are **test-only and intentionally insecure** — override them with
the `With*` options whenever a real, non-disposable instance is involved.

For example:

```go
app, err := paperlessngx.New(ctx, images.PaperlessNGX,
    paperlessngx.WithGotenberg(),
    paperlessngx.WithTika(),
)
```

## Tested versions

These live as versioned integration tests under
`applications/paperless-ngx/versions/`, one directory per version. The wrapper
pins all images so they move together:

| Component        | Image                                              |
|------------------|----------------------------------------------------|
| Paperless-ngx    | `index.docker.io/paperlessngx/paperless-ngx:3.1.3` |
| PostgreSQL       | `index.docker.io/library/postgres:18`              |
| Valkey           | `index.docker.io/valkey/valkey:9-alpine`           |
| Gotenberg (opt.) | `index.docker.io/gotenberg/gotenberg:8.34`         |
| Tika (opt.)      | `index.docker.io/apache/tika:3.3.1.0`              |

## Default image

There is no default image. `paperlessngx.New(ctx, image)` **requires an explicit
image** reference (the Paperless-ngx web server image), for example:

```text
index.docker.io/paperlessngx/paperless-ngx:3.1.3
```

The non-versioned tests pass `images.PaperlessNGX`; versioned tests pass their
pinned image.

## How to use

The first start runs database migrations and model/index setup against a fresh
PostgreSQL and can take several minutes (especially on hosts with slow disk I/O,
e.g. Docker Desktop on macOS). The wrapper's internal readiness timeout is
**10 minutes**, so pass a context of at least ~11–12 minutes:

```go
package main

import (
 "context"
 "fmt"
 "net/http"
 "time"

 "github.com/teran/go-docker-testsuite/applications/paperless-ngx"
 "github.com/teran/go-docker-testsuite/images"
)

func main() {
 ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
 defer cancel()

 app, err := paperlessngx.New(ctx, images.PaperlessNGX)
 if err != nil {
  panic(err)
 }
 defer app.Close(ctx)

 // Authenticate as the admin user with Basic auth.
 req, err := http.NewRequestWithContext(ctx, http.MethodGet, app.MustURL()+"/api/", nil)
 if err != nil {
  panic(err)
 }
 req.SetBasicAuth(app.Username(), app.Password())

 resp, err := http.DefaultClient.Do(req)
 if err != nil {
  panic(err)
 }
 defer resp.Body.Close()

 fmt.Println(resp.StatusCode) // 200 when authenticated
 _ = resp
}
```

Once Paperless-ngx is serving, an unauthenticated request to `<URL>` returns
`200`, while `<URL>/api/` with `Authorization: Basic base64(user:pass)` (via
`Username()` / `Password()`) returns `200`.

## Running the versioned tests

The versioned tests require a running Docker daemon:

```sh
go test ./applications/paperless-ngx/versions/...
```

You can target a single version, for example:

```sh
go test ./applications/paperless-ngx/versions/3.1.3/
```
