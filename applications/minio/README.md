# MinIO

S3-compatible object storage for integration testing. The `minio` wrapper
starts a single MinIO server (S3 API on port 9000 and a web console on port
9001) and returns a typed `Minio` interface exposing the endpoint URLs and
cleanup. You connect to it with the official
[`minio-go`](https://github.com/minio/minio-go) SDK.

## Tested versions

There are **no versioned integration tests** yet — `applications/minio/`
has no `versions/` directory. Only the default image below is used.

## Default image

When no explicit image is given, `New` uses the default from
[`images.Minio`](../../images/images.go):

```text
index.docker.io/pgsty/silo:RELEASE.2026-09-03T13-18-01Z
```

The default image is **PGSTY Silo**, a community-maintained fork of MinIO
([`pgsty/silo`](https://github.com/pgsty/silo)) that keeps publishing multi-arch
images and security fixes after upstream MinIO stopped distributing community
Docker images. It is wire- and on-disk compatible with MinIO, so the
`minio-go` client works unchanged.

Pass a custom image with `minio.NewWithImage(ctx, image)`.

## How to use

```go
package main

import (
    "context"
    "fmt"
    "time"

    minioSDK "github.com/minio/minio-go/v7"
    "github.com/minio/minio-go/v7/pkg/credentials"

    "github.com/teran/go-docker-testsuite/applications/minio"
)

func main() {
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
    defer cancel()

    app, err := minio.New(ctx)
    if err != nil {
        panic(err)
    }
    defer app.Close(ctx)

    endpoint, err := app.GetEndpointURL()
    if err != nil {
        panic(err)
    }

    cli, err := minioSDK.New(endpoint, &minioSDK.Options{
        Creds:  credentials.NewStaticV4(minio.MinioAccessKey, minio.MinioAccessKeySecret, ""),
        Secure: false,
    })
    if err != nil {
        panic(err)
    }

    if err := cli.MakeBucket(ctx, "example", minioSDK.MakeBucketOptions{}); err != nil {
        panic(err)
    }
    fmt.Println("bucket created")
}
```

The wrapper exposes `GetEndpointURL()` (S3 endpoint as `host:port`) and
`GetConsoleURL()` (web console). The default credentials are exported as
`minio.MinioAccessKey` and `minio.MinioAccessKeySecret` (both
`minioadmin`); they are injected into the container as the
`MINIO_ROOT_USER` / `MINIO_ROOT_PASSWORD` environment variables.

## Running the tests

There are no versioned integration tests for MinIO. The package-level test
and example exercise the default image and require a running Docker daemon:

```sh
go test ./applications/minio/...
```
