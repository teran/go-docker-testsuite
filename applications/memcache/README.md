# Memcache

Runs a Memcached container for integration testing and returns a
`memcache.Memcache` typed client. The wrapper starts the container, waits for
it to answer a ping, and exposes the endpoint address. You connect to it with
[`github.com/bradfitz/gomemcache/memcache`](https://pkg.go.dev/github.com/bradfitz/gomemcache/memcache).

The client interface provides `GetEndpointAddress()`, and `Close(ctx)`.

## Tested versions

There are no versioned integration tests yet — `applications/memcache/versions/`
does not exist. Only the default image below is covered by the package's
integration test (`applications/memcache/memcache_test.go`).

## Default image

When you call `memcache.New(ctx)` without an explicit image, the default
(`images.Memcache`) is used:

```text
index.docker.io/library/memcached:1.6.29-alpine3.20
```

Use `memcache.NewWithImage(ctx, image)` to select a different version.

## How to use

```go
package main

import (
 "context"
 "fmt"
 "time"

 memcacheCli "github.com/bradfitz/gomemcache/memcache"

 "github.com/teran/go-docker-testsuite/applications/memcache"
)

func main() {
 ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
 defer cancel()

 app, err := memcache.New(ctx)
 if err != nil {
  panic(err)
 }
 defer func() { _ = app.Close(ctx) }()

 addr, err := app.GetEndpointAddress()
 if err != nil {
  panic(err)
 }
 fmt.Println("memcache endpoint:", addr)

 cli := memcacheCli.New(addr)
 defer func() { _ = cli.Close() }()

 if err := cli.Set(&memcacheCli.Item{
  Key:   "greeting",
  Value: []byte("Hello, Memcache!"),
 }); err != nil {
  panic(err)
 }

 item, err := cli.Get("greeting")
 if err != nil {
  panic(err)
 }
 fmt.Printf("key value: %s\n", string(item.Value))
}
```

## Running the tests

There are no versioned tests for memcache yet. Run the package integration test
(it requires a running Docker daemon):

```sh
go test ./applications/memcache/
```
