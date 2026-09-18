package prometheus_test

import (
	"context"
	"fmt"
	"time"

	"github.com/teran/go-docker-testsuite/applications/prometheus"
)

// This example demonstrates starting a Prometheus container and querying its
// HTTP API for build information.
func Example() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	app, err := prometheus.New(ctx)
	if err != nil {
		fmt.Printf("error: %v (is Docker running?)\n", err)
		return
	}
	defer func() { _ = app.Close(ctx) }()

	bi, err := app.Client().Buildinfo(ctx)
	if err != nil {
		fmt.Printf("error querying buildinfo: %v\n", err)
		return
	}

	fmt.Printf("prometheus version: %s\n", bi.Version)
}
