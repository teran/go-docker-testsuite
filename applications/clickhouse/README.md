# applications/clickhouse

A typed wrapper around a [ClickHouse](https://clickhouse.com/) Docker container
for integration testing, using the
[`github.com/ClickHouse/clickhouse-go/v2`](https://github.com/ClickHouse/clickhouse-go)
client via the standard `database/sql` interface.

The wrapper manages the container lifecycle, waits for it to become ready
(probes with `clickhouse-client` inside the container, then driver ping), and
exposes:

- `DSN(db) (string, error)` / `MustDSN(db) string` — a
  `clickhouse://user:password@host:port/<db>` connection string.
- `CreateDatabase(ctx, name)` / `DropDatabase(ctx, name)` — DDL helpers
  (validated against an ASCII identifier whitelist).
- `Close(ctx)` — closes the client and stops the container.

`New(ctx)` uses the default image (`images.ClickHouse`), and
`NewWithImage(ctx, image)` starts a container with a specific image. The
corresponding T-bound variants `NewWithT(t, ctx)` and
`NewWithImageT(t, ctx, image)` bind the container lifecycle to the test via
`t.Cleanup`.

The wrapper sets `CLICKHOUSE_USER=default` and `CLICKHOUSE_PASSWORD=default` on
the container (ClickHouse disables network access for the `default` user unless
a user/password is provided) and embeds those credentials in the DSN.

## Usage

```go
app, err := clickhouse.New(ctx)
if err != nil {
    // handle error
}
defer func() { _ = app.Close(ctx) }()

if err := app.CreateDatabase(ctx, "mydb"); err != nil {
    // handle error
}

db, err := sql.Open("clickhouse", app.MustDSN("mydb"))
if err != nil {
    // handle error
}
defer func() { _ = db.Close() }()
```

## Versions tested

- `index.docker.io/clickhouse/clickhouse-server:24.8.1.2684`
- `index.docker.io/clickhouse/clickhouse-server:25.5.1`
