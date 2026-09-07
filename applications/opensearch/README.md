# OpenSearch

OpenSearch search engine for integration testing. The `opensearch` wrapper
starts a single-node OpenSearch server (HTTP on port 9200, security plugin
disabled) and returns a typed `OpenSearch` interface. It exposes the node
address and a ready-to-use
[`opensearch-go`](https://github.com/opensearch-project/opensearch-go) v4
client.

## Tested versions

The versioned integration tests live under
`applications/opensearch/versions/<version>/version_test.go`:

| Version | Image                                                        |
|---------|--------------------------------------------------------------|
| 2.12.0  | `index.docker.io/opensearchproject/opensearch:2.12.0`        |
| 2.17.1  | `index.docker.io/opensearchproject/opensearch:2.17.1`        |
| 2.19.6  | `index.docker.io/opensearchproject/opensearch:2.19.6`        |

## Default image

When no explicit image is given, `New` uses the default from
[`images.OpenSearch`](../../images/images.go):

```text
index.docker.io/opensearchproject/opensearch:2.19.6
```

Pass a custom image with `opensearch.NewWithImage(ctx, image)`.

## How to use

```go
package main

import (
    "context"
    "fmt"
    "strings"
    "time"

    "github.com/opensearch-project/opensearch-go/v4/opensearchapi"

    "github.com/teran/go-docker-testsuite/applications/opensearch"
)

func main() {
    ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
    defer cancel()

    app, err := opensearch.New(ctx)
    if err != nil {
        panic(err)
    }
    defer app.Close(ctx)

    client, err := app.Client()
    if err != nil {
        panic(err)
    }

    api := opensearchapi.NewFromClient(client)

    if _, err := api.Indices.Create(ctx, opensearchapi.IndicesCreateReq{
        Index: "my-index",
    }); err != nil {
        panic(err)
    }
    if _, err := api.Document.Create(ctx, opensearchapi.DocumentCreateReq{
        Index:      "my-index",
        DocumentID: "1",
        Body:       strings.NewReader(`{"message":"Hello, World!"}`),
    }); err != nil {
        panic(err)
    }
    fmt.Println("document indexed")
}
```

The wrapper exposes `Addr()` (node address as `host:port`), `MustAddr()`
(panics on error instead of returning it), and `Client()` which returns an
`opensearch-go` v4 client for the node.

## Running the versioned tests

The versioned integration tests require a running Docker daemon. Run them
all with:

```sh
go test ./applications/opensearch/versions/...
```

or a single version:

```sh
go test ./applications/opensearch/versions/2.19.6/
```
