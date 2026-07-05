package redis_test

import (
	"context"
	"fmt"
	"time"

	redisClient "github.com/go-redis/redis/v8"

	"github.com/teran/go-docker-testsuite/applications/redis"
)

// This example demonstrates starting a Redis container, connecting via
// go-redis, and performing SET/GET operations.
func Example() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	app, err := redis.New(ctx, "index.docker.io/library/redis:7.2")
	if err != nil {
		fmt.Printf("error: %v (is Docker running?)\n", err)
		return
	}
	defer app.Close(ctx)

	rdb := redisClient.NewClient(&redisClient.Options{
		Addr: app.MustAddr(),
	})
	defer rdb.Close()

	if err := rdb.Set(ctx, "mykey", "Hello, World!", 0).Err(); err != nil {
		fmt.Printf("error setting key: %v\n", err)
		return
	}
	fmt.Println("key set")

	val, err := rdb.Get(ctx, "mykey").Result()
	if err != nil {
		fmt.Printf("error getting key: %v\n", err)
		return
	}
	fmt.Printf("key value: %s\n", val)
}
