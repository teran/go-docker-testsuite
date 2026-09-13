package netbox_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/teran/go-docker-testsuite/applications/netbox"
	"github.com/teran/go-docker-testsuite/images"
)

// This example demonstrates starting a NetBox instance (with its backing
// PostgreSQL and Redis containers) and fetching its API root over HTTP using
// the superuser API token.
func Example() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	app, err := netbox.New(ctx, images.NetBox)
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
	// NetBox 4.6 authenticates v2 API tokens with the "Bearer" scheme.
	req.Header.Set("Authorization", "Bearer "+app.SuperuserAPIToken())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	_, _ = io.Copy(io.Discard, resp.Body)
	fmt.Printf("status code: %d\n", resp.StatusCode)
}
