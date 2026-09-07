# Ceph (RGW)

Ceph object storage for integration testing. The `ceph` wrapper starts a
single-container Ceph demo cluster (mon, mgr, osd and RGW) and returns a
typed `Ceph` interface. It exposes an S3-compatible endpoint plus the demo
credentials, and can hand you a ready-to-use
[AWS SDK v2](https://github.com/aws/aws-sdk-go-v2) S3 client configured for
the RGW.

## Tested versions

The versioned integration tests live under
`applications/ceph/versions/<version>/version_test.go` and cover the
`squid` and `tentacle` release trains:

| Version | Image                                          |
|---------|------------------------------------------------|
| 19.2.0  | `ghcr.io/teran/ceph-container/ceph:v19.2.0`    |
| 19.2.1  | `ghcr.io/teran/ceph-container/ceph:v19.2.1`    |
| 19.2.2  | `ghcr.io/teran/ceph-container/ceph:v19.2.2`    |
| 19.2.3  | `ghcr.io/teran/ceph-container/ceph:v19.2.3`    |
| 19.2.4  | `ghcr.io/teran/ceph-container/ceph:v19.2.4`    |
| 19.2.5  | `ghcr.io/teran/ceph-container/ceph:v19.2.5`    |
| 19.2.6  | `ghcr.io/teran/ceph-container/ceph:v19.2.6`    |
| 20.2.0  | `ghcr.io/teran/ceph-container/ceph:v20.2.0`    |
| 20.2.1  | `ghcr.io/teran/ceph-container/ceph:v20.2.1`    |
| 20.2.2  | `ghcr.io/teran/ceph-container/ceph:v20.2.2`    |
| 20.2.3  | `ghcr.io/teran/ceph-container/ceph:v20.2.3`    |
| 20.2.4  | `ghcr.io/teran/ceph-container/ceph:v20.2.4`    |

These images are published multi-arch (amd64 + arm64) as
`ghcr.io/teran/ceph-container/ceph:v<version>` and are built and released
independently from [github.com/teran/ceph-container](https://github.com/teran/ceph-container).

## Default image

When no explicit image is given, `New` uses the default from
[`images.Ceph`](../../images/images.go):

```text
ghcr.io/teran/ceph-container/ceph:v20.2.4
```

Pick any published version with `ceph.NewWithImage(ctx, image)`.

## How to use

```go
package main

import (
    "context"
    "fmt"
    "strings"
    "time"

    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/service/s3"

    "github.com/teran/go-docker-testsuite/applications/ceph"
)

func main() {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
    defer cancel()

    app, err := ceph.New(ctx)
    if err != nil {
        panic(err)
    }
    defer app.Close(ctx)

    cli, err := app.Client()
    if err != nil {
        panic(err)
    }

    if _, err := cli.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String("example")}); err != nil {
        panic(err)
    }
    if _, err := cli.PutObject(ctx, &s3.PutObjectInput{
        Bucket: aws.String("example"),
        Key:    aws.String("hello.txt"),
        Body:   strings.NewReader("Hello, Ceph!"),
    }); err != nil {
        panic(err)
    }
    fmt.Println("object uploaded")
}
```

The wrapper exposes `Endpoint()` (RGW S3 endpoint as `host:port`),
`AccessKey()` / `SecretKey()` (the demo credentials, exported as
`ceph.DefaultAccessKey` / `ceph.DefaultSecretKey`, both `access` / `secret`),
and `Client()` which returns an AWS SDK v2 S3 client configured for the RGW
(path-style addressing).

## Running the versioned tests

The versioned integration tests require a running Docker daemon. Run them
all with:

```sh
go test ./applications/ceph/versions/...
```

or a single version:

```sh
go test ./applications/ceph/versions/20.2.4/
```

The package-level test uses `images.Ceph` by default and can be pointed at
any published image via the `CEPH_IMAGE` environment variable.
