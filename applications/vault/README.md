# Vault

Runs a [HashiCorp Vault](https://www.vaultproject.io/) server in a Docker
container for integration testing. The wrapper waits for Vault to become ready
and exposes a typed `vault.Vault` interface from which you can retrieve the
root token and a pre-authenticated `*vault.Client`
([hashicorp/vault-client-go](https://github.com/hashicorp/vault-client-go)),
and create/remove secrets engines.

## Tested versions

There are **no versioned integration tests** for Vault yet — there is no
`applications/vault/versions/` directory. The wrapper accepts any Vault image
via its constructor; the image used by the package's own tests and examples is:

```go
index.docker.io/hashicorp/vault:1.21.0
```

## Default image

There is **no default image** — `vault.New` requires the image as an explicit
argument (unlike wrappers that default to an `images.*` constant).

## How to use

```go
package main

import (
 "context"
 "fmt"
 "time"

 vaultSDK "github.com/hashicorp/vault-client-go"
 "github.com/hashicorp/vault-client-go/schema"

 "github.com/teran/go-docker-testsuite/applications/vault"
)

func main() {
 ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
 defer cancel()

 app, err := vault.New(ctx, "index.docker.io/hashicorp/vault:1.21.0")
 if err != nil {
  panic(err)
 }
 defer func() { _ = app.Close(ctx) }()

 rootToken, err := app.GetRootToken(ctx)
 if err != nil {
  panic(err)
 }
 fmt.Println("vault started, root token:", rootToken)

 if err := app.CreateEngine(ctx, "mysecrets", "kv-v2"); err != nil {
  panic(err)
 }

 client, err := app.GetRootClient(ctx)
 if err != nil {
  panic(err)
 }

 if _, err := client.Secrets.KvV2Write(ctx, "config", schema.KvV2WriteRequest{
  Data: map[string]any{"key": "value"},
 }, vaultSDK.WithMountPath("mysecrets")); err != nil {
  panic(err)
 }

 sec, err := client.Secrets.KvV2Read(ctx, "config", vaultSDK.WithMountPath("mysecrets"))
 if err != nil {
  panic(err)
 }
 fmt.Printf("secret value: %s\n", sec.Data.Data["key"])

 if err := app.RemoveEngine(ctx, "mysecrets"); err != nil {
  panic(err)
 }
}
```

`ClusterAddr()` and `APIAddr()` return the published HTTP addresses for the
cluster (port 8200) and API (port 8201) respectively, should you need to
connect outside the returned client.

## Running the tests

Requires a running Docker daemon:

```sh
go test ./applications/vault/...
```
