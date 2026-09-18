# applications/prometheus

A typed wrapper around a [Prometheus](https://prometheus.io/) Docker container
for integration testing, using the
[`github.com/prometheus/client_golang`](https://github.com/prometheus/client_golang)
client for the Prometheus HTTP API (v1).

The wrapper manages the container lifecycle, waits for it to become ready
(HTTP GET on `/-/ready`), and exposes:

- `Client()` — a `promv1.API` client for querying the Prometheus HTTP API
  (`Query`, `QueryRange`, `Targets`, `Alerts`, `Buildinfo`, etc.).
- `Close(ctx)` — stops the container.

`New(ctx)` uses the default image (`images.Prometheus`), and
`NewWithImage(ctx, image)` starts a container with a specific image. The
corresponding T-bound variants `NewWithT(t, ctx)` and
`NewWithImageT(t, ctx, image)` bind the container lifecycle to the test via
`t.Cleanup`.

## Usage

```go
app, err := prometheus.New(ctx)
if err != nil {
    // handle error
}
defer func() { _ = app.Close(ctx) }()

bi, err := app.Client().Buildinfo(ctx)
if err != nil {
    // handle error
}
fmt.Println(bi.Version)
```

## Versions tested

- `index.docker.io/prom/prometheus:v2.55.1`
- `index.docker.io/prom/prometheus:v3.1.0`
