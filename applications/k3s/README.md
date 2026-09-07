# K3s

Runs a [K3s](https://k3s.io/) Kubernetes cluster in a Docker container for
integration testing. The wrapper starts K3s (with traefik, the metrics server
and local storage disabled), waits for it to become ready, retrieves the
kubeconfig from inside the container and rewrites its API server address so it
points at the published port. It returns a typed `k3s.K3s` interface from which
you obtain a standard `*kubernetes.Clientset` (client-go) connected to the
cluster.

## Tested versions

The versioned integration tests live under
[`applications/k3s/versions/`](./versions/) — one package per K3s image tag.
Each verifies that the container starts and that the reported Kubernetes server
version minor (e.g. `v1.36`) matches the minor embedded in the image tag.

| Version   | Image                                          |
|-----------|------------------------------------------------|
| v1.31.14  | `index.docker.io/rancher/k3s:v1.31.14-k3s1`    |
| v1.32.13  | `index.docker.io/rancher/k3s:v1.32.13-k3s1`    |
| v1.33.13  | `index.docker.io/rancher/k3s:v1.33.13-k3s1`    |
| v1.34.9   | `index.docker.io/rancher/k3s:v1.34.9-k3s1`     |
| v1.35.6   | `index.docker.io/rancher/k3s:v1.35.6-k3s1`     |
| v1.36.2   | `index.docker.io/rancher/k3s:v1.36.2-k3s1`     |

## Default image

When no image is given, `k3s.New` uses the default image from
`images.K3s`:

```go
index.docker.io/rancher/k3s:v1.36.2-k3s1
```

Use `NewWithImage` to run a specific K3s image instead.

## How to use

```go
package main

import (
 "context"
 "fmt"
 "time"

 metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

 "github.com/teran/go-docker-testsuite/applications/k3s"
)

func main() {
 ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
 defer cancel()

 app, err := k3s.New(ctx)
 if err != nil {
  panic(err)
 }
 defer func() { _ = app.Close(ctx) }()

 cs, err := app.Clientset(ctx)
 if err != nil {
  panic(err)
 }

 nodes, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
 if err != nil {
  panic(err)
 }
 fmt.Printf("k3s cluster has %d node(s)\n", len(nodes.Items))

 sv, err := cs.Discovery().ServerVersion()
 if err != nil {
  panic(err)
 }
 fmt.Printf("k3s cluster ready: %s\n", sv.GitVersion)
}
```

You can also use `KubeconfigPath()` to get the path to the rewritten kubeconfig
file if you need to point other tooling at the cluster.

## Running the versioned tests

Requires a running Docker daemon:

```sh
go test ./applications/k3s/versions/...
```
