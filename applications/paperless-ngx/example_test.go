package paperlessngx_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/teran/go-docker-testsuite/applications/paperless-ngx"
	"github.com/teran/go-docker-testsuite/images"
)

// This example demonstrates starting a Paperless-ngx instance (with its backing
// PostgreSQL and Valkey containers) and fetching its API root over HTTP using
// the admin credentials.
func Example() {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	app, err := paperlessngx.New(ctx, images.PaperlessNGX)
	if err != nil {
		fmt.Printf("error: %v (is Docker running?)\n", err)
		return
	}
	defer func() { _ = app.Close(ctx) }()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, app.MustURL()+"/api/", nil)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}
	req.SetBasicAuth(app.Username(), app.Password())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	_, _ = io.Copy(io.Discard, resp.Body)
	fmt.Printf("status code: %d\n", resp.StatusCode)
}
