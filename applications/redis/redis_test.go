package redis

import (
	"context"
	"testing"

	log "github.com/sirupsen/logrus"

	redisClient "github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
)

func init() {
	log.SetLevel(log.TraceLevel)
}

func TestRedis(t *testing.T) {
	r := require.New(t)

	ctx := context.Background()

	app, err := New(ctx)
	r.NoError(err)

	defer app.Close(ctx)

	rdb := redisClient.NewClient(&redisClient.Options{
		Addr:     app.MustAddr(),
		Password: "",
		DB:       0,
	})

	err = rdb.Set(ctx, "key", "value", 0).Err()
	r.NoError(err)

	val, err := rdb.Get(ctx, "key").Result()
	r.NoError(err)
	r.Equal("value", val)
}
