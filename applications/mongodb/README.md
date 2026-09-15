# applications/mongodb

A typed wrapper around a [MongoDB](https://www.mongodb.com/) Docker container
for integration testing, using the official
[`go.mongodb.org/mongo-driver`](https://github.com/mongodb/mongo-go-driver)
client (v1.17.10).

The wrapper manages the container lifecycle, waits for it to become ready
(log line + driver ping), and exposes:

- `URI(db) (string, error)` / `MustURI(db) string` — a
  `mongodb://host:port/<db>` connection string.
- `CreateDatabase(ctx, name)` / `DropDatabase(ctx, name)` — DDL helpers.
  MongoDB has no `CREATE DATABASE` statement; `CreateDatabase` materializes a
  database by creating a collection on it via the driver API (injection-safe),
  and `DropDatabase` drops it.
- `Close(ctx)` — disconnects the client and stops the container.

Both `New(ctx, image)` and `NewWithT(t, ctx, image)` constructors are provided;
`NewWithT` binds the container lifecycle to the test via `t.Cleanup`.

## Usage

```go
app, err := mongodb.New(ctx, images.MongoDB)
if err != nil {
    // handle error
}
defer func() { _ = app.Close(ctx) }()

client, err := mongo.Connect(ctx, options.Client().ApplyURI(app.MustURI("mydb")))
if err != nil {
    // handle error
}
defer func() { _ = client.Disconnect(ctx) }()

_, err = client.Database("mydb").Collection("items").InsertOne(
    ctx, bson.M{"name": "hello"},
)
```

## Versions tested

- `index.docker.io/library/mongo:7.0.43`
- `index.docker.io/library/mongo:8.0.4`

## Known limitations

Newer MongoDB 8.x images may refuse to start on Linux kernels **6.19 and
newer** because of a known incompatibility — mongod exits immediately with:

> MongoDB cannot start: Linux kernel versions 6.19 and newer has a known
> incompatibility with this version of MongoDB. See
> <https://jira.mongodb.org/browse/SERVER-121912> for more information.

This affects, for example, `mongo:8.0.32` and the 8.1–8.3 line. Docker Desktop
for macOS runs a LinuxKit VM whose kernel is ≥ 6.19, so the newest images
cannot run locally. The wrapper therefore pins `mongo:8.0.4` — the newest 8.x
that starts on such kernels — and stays on the 7.0.43 / 8.0.4 pair for the
versioned integration tests.
