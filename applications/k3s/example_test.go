package k3s_test

import (
	"context"
	"fmt"
	"time"

	"github.com/teran/go-docker-testsuite/applications/k3s"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// This example demonstrates starting a K3s cluster container, obtaining a
// Kubernetes clientset, listing nodes and the server version.
func Example() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	app, err := k3s.New(ctx)
	if err != nil {
		fmt.Printf("error: %v (is Docker running?)\n", err)
		return
	}
	defer func() { _ = app.Close(ctx) }()

	cs, err := app.Clientset(ctx)
	if err != nil {
		fmt.Printf("error creating clientset: %v\n", err)
		return
	}

	nodes, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		fmt.Printf("error listing nodes: %v\n", err)
		return
	}
	fmt.Printf("k3s cluster has %d node(s)\n", len(nodes.Items))

	sv, err := cs.Discovery().ServerVersion()
	if err != nil {
		fmt.Printf("error getting server version: %v\n", err)
		return
	}
	fmt.Printf("k3s cluster ready: %s\n", sv.GitVersion)
}
