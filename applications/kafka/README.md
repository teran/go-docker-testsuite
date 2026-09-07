# Kafka

Runs an Apache Kafka container for integration testing and returns a
`kafka.Kafka` typed client. The wrapper starts a single-node Kafka broker in
combined broker/controller (KRaft) mode, waits for it to become ready, and
exposes the broker and admin endpoints. You connect to it with
[`github.com/IBM/sarama`](https://pkg.go.dev/github.com/IBM/sarama).

The client interface provides `GetBrokerURL(ctx)`, `GetAdminURL(ctx)`, and
`Close(ctx)`.

## Tested versions

These live as versioned integration tests under
`applications/kafka/versions/`, one directory per version:

| Version | Image                                |
|---------|--------------------------------------|
| 4.1.2   | `index.docker.io/apache/kafka:4.1.2` |
| 4.2.1   | `index.docker.io/apache/kafka:4.2.1` |
| 4.3.1   | `index.docker.io/apache/kafka:4.3.1` |

## Default image

When you call `kafka.New(ctx)` without an explicit image, the default
(`images.Kafka`) is used:

```text
index.docker.io/apache/kafka:4.0.0
```

Use `kafka.NewWithImage(ctx, image)` to select a different version.

## How to use

```go
package main

import (
 "context"
 "fmt"
 "time"

 "github.com/IBM/sarama"

 "github.com/teran/go-docker-testsuite/applications/kafka"
)

func main() {
 ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
 defer cancel()

 app, err := kafka.New(ctx)
 if err != nil {
  panic(err)
 }
 defer func() { _ = app.Close(ctx) }()

 brokerURL, err := app.GetBrokerURL(ctx)
 if err != nil {
  panic(err)
 }
 fmt.Println("kafka broker:", brokerURL)

 cfg := sarama.NewConfig()
 cfg.Version = sarama.V4_0_0_0
 cfg.Producer.Return.Successes = true

 producer, err := sarama.NewSyncProducer([]string{brokerURL}, cfg)
 if err != nil {
  panic(err)
 }
 defer func() { _ = producer.Close() }()

 if _, _, err := producer.SendMessage(&sarama.ProducerMessage{
  Topic: "events",
  Value: sarama.StringEncoder("hello"),
 }); err != nil {
  panic(err)
 }
}
```

## Running the versioned tests

The versioned tests require a running Docker daemon:

```sh
go test ./applications/kafka/versions/...
```

You can target a single version, for example:

```sh
go test ./applications/kafka/versions/4.3.1/
```
