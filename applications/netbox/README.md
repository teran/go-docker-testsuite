# NetBox

Runs a NetBox (DCIM/IPAM) instance for integration testing and returns a
`netbox.NetBox` typed handle. Unlike most other application wrappers, NetBox
cannot run on SQLite — it requires an external PostgreSQL database and a Redis
cache — so this is the first **multi-container `docker.Group`-based** wrapper in
the library. It spins up three containers on a shared internal network where the
siblings resolve each other by container name:

- `netbox-postgres` (PostgreSQL, internal only)
- `netbox-redis` (Redis, internal only)
- `netbox` (the NetBox web app, port `8080` DNAT'd to the host)

Only the NetBox web container's `:8080` is published; the PostgreSQL and Redis
dependencies stay internal and are never exposed to the host.

The client interface provides `URL()` / `MustURL()` (the web UI / REST API root,
resolved to its own random host port), `SuperuserUsername()` /
`SuperuserPassword()` (the credentials of the superuser created during setup),
`SuperuserAPIToken()` (the full NetBox v2 API credential, used as
`Authorization: Bearer <token>`), and `Close(ctx)`.

No NetBox SDK dependency is embedded: this wrapper is deliberately stdlib-only,
and the caller brings their own HTTP client (e.g. `net/http`) — the wrapper only
hands back the URL and credentials.

## Options

Constructors accept optional `Option`s (variadic):

- `netbox.WithSecretKey("...")` — sets the Django `SECRET_KEY`. Defaults to a
  fixed, deterministic and deliberately insecure test-only value.
- `netbox.WithAllowedHosts("a.example", "b.example")` — sets `ALLOWED_HOSTS`
  (space-joined into a single env var). Defaults to `["*"]` so the instance
  answers on any host.
- `netbox.WithDBPassword("...")` — sets the password for the backing PostgreSQL
  (`POSTGRES_PASSWORD` / `DB_PASSWORD`). Defaults to a fixed test-only constant.
- `netbox.WithSuperuser("user", "pass", "token", "user@example.com")` —
  overrides the superuser created during setup (`SUPERUSER_NAME` /
  `SUPERUSER_PASSWORD` / `SUPERUSER_API_TOKEN` / `SUPERUSER_EMAIL`). The
  `apiToken` argument is the plaintext of the NetBox v2 API token; the wrapper
  builds the full `nbt_<key>.<plaintext>` credential from it. Defaults to a
  deterministic test account (`admin`).

All defaults are **test-only and intentionally insecure** — override them with
the `With*` options whenever a real, non-disposable instance is involved.

For example:

```go
app, err := netbox.New(ctx, "index.docker.io/netboxcommunity/netbox:v4.6-5.0.1",
    netbox.WithSecretKey("a-real-secret"),
)
```

## Tested versions

These live as versioned integration tests under
`applications/netbox/versions/`, one directory per version. The wrapper pins
all three images so they move together with the netbox-docker release:

| Component   | Image                                               |
|-------------|-----------------------------------------------------|
| NetBox      | `index.docker.io/netboxcommunity/netbox:v4.6-5.0.1` |
| PostgreSQL  | `index.docker.io/library/postgres:16.15`            |
| Redis       | `index.docker.io/library/redis:7.4-alpine`          |

## Default image

There is no default image. `netbox.New(ctx, image)` **requires an explicit
image** reference, for example:

```text
index.docker.io/netboxcommunity/netbox:v4.6-5.0.1
```

## How to use

The first start runs Django's ~200 database migrations against a fresh
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

 "github.com/teran/go-docker-testsuite/applications/netbox"
)

func main() {
 ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
 defer cancel()

 app, err := netbox.New(ctx, "index.docker.io/netboxcommunity/netbox:v4.6-5.0.1")
 if err != nil {
  panic(err)
 }
 defer app.Close(ctx)

 // Authenticate as the superuser with the v2 API token.
 req, err := http.NewRequestWithContext(ctx, http.MethodGet, app.MustURL()+"/api/", nil)
 if err != nil {
  panic(err)
 }
 req.Header.Set("Authorization", "Bearer "+app.SuperuserAPIToken())

 resp, err := http.DefaultClient.Do(req)
 if err != nil {
  panic(err)
 }
 defer resp.Body.Close()

 fmt.Println(resp.StatusCode) // 200 when authenticated
 _ = resp
}
```

Once NetBox is serving, an unauthenticated request to `<URL>/api/` returns
`403`, while one carrying `Authorization: Bearer <SuperuserAPIToken()>` returns
`200`.

## Running the versioned tests

The versioned tests require a running Docker daemon:

```sh
go test ./applications/netbox/versions/...
```

You can target a single version, for example:

```sh
go test ./applications/netbox/versions/v4.6-5.0.1/
```
