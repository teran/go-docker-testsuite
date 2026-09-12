package forgejo_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/teran/go-docker-testsuite/applications/forgejo"
)

// This example demonstrates starting a Forgejo container and fetching its web
// UI root over HTTP.
func Example() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	app, err := forgejo.New(ctx, "codeberg.org/forgejo/forgejo:16")
	if err != nil {
		fmt.Printf("error: %v (is Docker running?)\n", err)
		return
	}
	defer func() { _ = app.Close(ctx) }()

	resp, err := http.Get(app.MustURL())
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	_, _ = io.Copy(io.Discard, resp.Body)
	fmt.Printf("status code: %d\n", resp.StatusCode)
}
