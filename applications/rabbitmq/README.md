# RabbitMQ

Runs a RabbitMQ container (with the management plugin) for integration testing
and returns a `rabbitmq.RabbitMQ` typed client. The wrapper starts the broker,
waits for it to become ready, and exposes the AMQP and management endpoints
plus helpers to provision virtual hosts, users, and permissions via the
management API. You connect to it with
[`github.com/rabbitmq/amqp091-go`](https://pkg.go.dev/github.com/rabbitmq/amqp091-go).

The client interface provides `GetAMQPURL(ctx)`, `GetManagementURL(ctx)`,
`CreateVHost(ctx, name)`, `CreateUser(ctx, username, password)`,
`SetPermissions(ctx, vhost, username, configure, write, read)`, and
`Close(ctx)`.

## Tested versions

These live as versioned integration tests under
`applications/rabbitmq/versions/`, one directory per version:

| Version | Image                                              |
|---------|----------------------------------------------------|
| 3.13    | `index.docker.io/library/rabbitmq:3.13-management` |
| 4.0     | `index.docker.io/library/rabbitmq:4.0-management`  |

## Default image

When you call `rabbitmq.New(ctx)` without an explicit image, the default
(`images.RabbitMQ`) is used:

```text
index.docker.io/library/rabbitmq:4.0-management
```

Use `rabbitmq.NewWithImage(ctx, image)` to select a different version.

## How to use

```go
package main

import (
 "context"
 "time"

 amqp "github.com/rabbitmq/amqp091-go"

 "github.com/teran/go-docker-testsuite/applications/rabbitmq"
)

func main() {
 ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
 defer cancel()

 app, err := rabbitmq.New(ctx)
 if err != nil {
  panic(err)
 }
 defer func() { _ = app.Close(ctx) }()

 // Provision a vhost and a dedicated user via the management API.
 if err := app.CreateVHost(ctx, "/example"); err != nil {
  panic(err)
 }
 if err := app.CreateUser(ctx, "example-user", "example-pass"); err != nil {
  panic(err)
 }
 if err := app.SetPermissions(ctx, "/example", "example-user", ".*", ".*", ".*"); err != nil {
  panic(err)
 }

 amqpURL, err := app.GetAMQPURL(ctx)
 if err != nil {
  panic(err)
 }

 conn, err := amqp.Dial(amqpURL)
 if err != nil {
  panic(err)
 }
 defer func() { _ = conn.Close() }()
}
```

## Running the versioned tests

The versioned tests require a running Docker daemon:

```sh
go test ./applications/rabbitmq/versions/...
```

You can target a single version, for example:

```sh
go test ./applications/rabbitmq/versions/4.0/
```
