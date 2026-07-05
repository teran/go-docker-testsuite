package memcache_test

import (
	"context"
	"fmt"
	"time"

	memcacheCli "github.com/bradfitz/gomemcache/memcache"

	"github.com/teran/go-docker-testsuite/applications/memcache"
)

// This example demonstrates starting a Memcache container, connecting via
// gomemcache, and performing SET/GET operations.
func Example() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	app, err := memcache.New(ctx)
	if err != nil {
		fmt.Printf("error: %v (is Docker running?)\n", err)
		return
	}
	defer app.Close(ctx)

	addr, err := app.GetEndpointAddress()
	if err != nil {
		fmt.Printf("error getting address: %v\n", err)
		return
	}
	fmt.Println("memcache endpoint:", addr)

	cli := memcacheCli.New(addr)
	defer cli.Close()

	if err := cli.Set(&memcacheCli.Item{
		Key:   "greeting",
		Value: []byte("Hello, Memcache!"),
	}); err != nil {
		fmt.Printf("error setting key: %v\n", err)
		return
	}
	fmt.Println("key set")

	item, err := cli.Get("greeting")
	if err != nil {
		fmt.Printf("error getting key: %v\n", err)
		return
	}
	fmt.Printf("key value: %s\n", string(item.Value))
}
