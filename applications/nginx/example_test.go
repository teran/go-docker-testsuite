package nginx_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/teran/go-docker-testsuite/applications/nginx"
)

// Example demonstrates starting an nginx container with an injected config and
// fetching a response from it.
func Example() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	config := []byte(`server {
    listen 80;
    location / {
        return 200 "hello-from-nginx";
    }
}
`)

	app, err := nginx.NewWithConfig(ctx, config)
	if err != nil {
		fmt.Printf("error: %v (is Docker running?)\n", err)
		return
	}
	defer func() { _ = app.Close(ctx) }()

	addr := app.MustAddr()
	fmt.Println("nginx started:", addr)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://" + addr + "/")
	if err != nil {
		fmt.Printf("error fetching: %v\n", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	data, _ := io.ReadAll(resp.Body)
	fmt.Printf("response: %s\n", string(data))
}
