# Redis

Runs a Redis container for integration testing and returns a `redis.Redis`
typed client. The wrapper starts the container, waits for Redis to accept
connections, and exposes helpers to obtain the server address, which you then
feed to your Redis client of choice (e.g.
[`github.com/go-redis/redis/v8`](https://github.com/go-redis/redis)).

The client interface provides `Addr()` / `MustAddr()` and `Close(ctx)`.

## Tested versions

These live as versioned integration tests under
`applications/redis/versions/`, one directory per version:

| Version | Image                                  |
|---------|----------------------------------------|
| 6.2.14  | `index.docker.io/library/redis:6.2.14` |
| 7.0.15  | `index.docker.io/library/redis:7.0.15` |
| 7.2.5   | `index.docker.io/library/redis:7.2.5`  |

## Default image

There is no default image. `redis.New(ctx, image)` **requires an explicit image**
reference, for example:

```text
index.docker.io/library/redis:7.2
```

## How to use

```go
package main

import (
 "context"
 "time"

 redisClient "github.com/go-redis/redis/v8"

 "github.com/teran/go-docker-testsuite/applications/redis"
)

func main() {
 ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
 defer cancel()

 app, err := redis.New(ctx, "index.docker.io/library/redis:7.2")
 if err != nil {
  panic(err)
 }
 defer app.Close(ctx)

 rdb := redisClient.NewClient(&redisClient.Options{
  Addr: app.MustAddr(),
 })
 defer rdb.Close()

 if err := rdb.Set(ctx, "mykey", "Hello, World!", 0).Err(); err != nil {
  panic(err)
 }

 val, err := rdb.Get(ctx, "mykey").Result()
 if err != nil {
  panic(err)
 }
 _ = val
}
```

## Running the versioned tests

The versioned tests require a running Docker daemon:

```sh
go test ./applications/redis/versions/...
```

You can target a single version, for example:

```sh
go test ./applications/redis/versions/7.2.5/
```
