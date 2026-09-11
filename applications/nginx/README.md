# Nginx

Runs an nginx container for integration testing. nginx is typically used as a
reverse proxy in front of the service under test, or as a configurable web
server for arbitrary routing (e.g. `auth_request`). The wrapper injects your
config verbatim into `/etc/nginx/conf.d/default.conf` before the container
starts, using the core `WithFiles` file-seeding mechanism.

The `nginx.Nginx` interface exposes `Addr()` / `MustAddr()` (the reachable
`host:port` of the HTTP listener), `Container()` (for `Group` membership) and
`Close(ctx)`.

## Tested versions

The versioned integration tests live under
`applications/nginx/versions/`, one directory per version:

| Version | Image                                             |
|---------|---------------------------------------------------|
| 1.27    | `index.docker.io/library/nginx:1.27-alpine`       |

## Default image

When no image is given, the wrapper uses the default from
[`images.Nginx`](../../images/images.go):

```text
index.docker.io/library/nginx:1.27-alpine
```

Pass a custom image with `WithImage`.

## How to use

The wrapper offers two constructors:

- `NewWithConfig(ctx, config, opts...)` — arbitrary nginx `server { ... }`
  config, bridge networking by default.
- `NewReverseProxyHost(ctx, upstreamHostPort, opts...)` — reverse-proxy every
  request to a service listening on the host loopback; host networking.

### `NewWithConfig` (bridge default)

`NewWithConfig` injects `config` verbatim into
`/etc/nginx/conf.d/default.conf` and runs nginx on the Docker bridge (the
default is `WithNetworkMode(docker.NetworkModeBridge)`), so it can also be
joined to a `Group` and proxy to a sibling container by name. Use it for
full-control cases such as `auth_request` or custom `location`/`server`
routing:

```go
package main

import (
    "context"
    "fmt"
    "io"
    "net/http"
    "time"

    "github.com/teran/go-docker-testsuite/applications/nginx"
)

func main() {
    ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
    defer cancel()

    config := []byte(`server {
    listen 80;
    location / {
        return 200 "hello-from-nginx";
    }
}
`)

    app, err := nginx.NewWithConfig(ctx, config)
    if err != nil {
        panic(err)
    }
    defer app.Close(ctx)

    resp, err := http.Get("http://" + app.MustAddr() + "/")
    if err != nil {
        panic(err)
    }
    defer resp.Body.Close()

    body, _ := io.ReadAll(resp.Body)
    fmt.Println(string(body)) // "hello-from-nginx"
}
```

### `NewReverseProxyHost` (host network)

`NewReverseProxyHost` is the primary use case: reverse-proxy every request to a
server already listening on the host loopback at
`127.0.0.1:<upstreamHostPort>` (the tested application, typically started in a
goroutine in the test). Because nginx must reach the host loopback it runs
with host networking (`docker.WithHostNetwork()`), and nginx binds a random
free host port (or the one given via `WithListenPort`). The generated config
is:

```text
server {
    listen <nginxListenPort>;
    location / {
        proxy_pass http://127.0.0.1:<upstreamHostPort>;
    }
}
```

`Addr()` returns `127.0.0.1:<nginxListenPort>`.

```go
package main

import (
    "context"
    "fmt"
    "io"
    "net/http"
    "time"

    "github.com/teran/go-docker-testsuite/applications/nginx"
)

func main() {
    ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
    defer cancel()

    // Start the tested server on the host loopback, e.g. in a goroutine.
    upstream := &http.Server{
        Addr: "127.0.0.1:8080",
        Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            _, _ = io.WriteString(w, "hello-from-upstream")
        }),
    }
    go upstream.ListenAndServe()
    defer upstream.Close()

    app, err := nginx.NewReverseProxyHost(ctx, 8080)
    if err != nil {
        panic(err)
    }
    defer app.Close(ctx)

    resp, err := http.Get("http://" + app.MustAddr() + "/")
    if err != nil {
        panic(err)
    }
    defer resp.Body.Close()

    body, _ := io.ReadAll(resp.Body)
    fmt.Println(string(body)) // "hello-from-upstream"
}
```

> **Platform caveat.** Host networking shares the real host network namespace
> only under native Linux Docker. On Docker Desktop (macOS/Windows) the
> containers run inside a VM, so the host-net path to the host loopback is not
> reachable from the container and the reverse proxy is verified by inspection
> only.

### Options

- `WithNetworkMode(mode docker.NetworkMode)` — sets the container network
  mode (`NewWithConfig` defaults to bridge; ignored in
  `NewReverseProxyHost`, which forces host).
- `WithListenPort(port uint16)` — the port nginx listens on. Under bridge
  networking this is the internal (unmapped) port, default `80` (also DNAT'd);
  under host networking it is the host port nginx binds directly, defaulting
  to a random free port allocated at construction.
- `WithImage(image string)` — overrides the nginx image (used by versioned
  integration tests).

### `*testing.T` variants

`NewWithConfigT(t, ctx, config, opts...)` and
`NewReverseProxyHostT(t, ctx, upstreamHostPort, opts...)` bind the container's
lifecycle to a `*testing.T` and clean it up automatically via `t.Cleanup`.

## Running the tests

The integration tests require a running Docker daemon:

```sh
go test ./applications/nginx/...
```

or the versioned suite:

```sh
go test ./applications/nginx/versions/...
```
