package memcache

import (
	"context"
	"testing"
	"time"

	memcacheCli "github.com/bradfitz/gomemcache/memcache"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func init() {
	log.SetLevel(log.TraceLevel)
}

func TestMemcache(t *testing.T) {
	r := require.New(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	app, err := New(ctx)
	r.NoError(err)

	defer func() { _ = app.Close(ctx) }()

	e, err := app.GetEndpointAddress()
	r.NoError(err)

	c := memcacheCli.New(e)
	err = c.Set(&memcacheCli.Item{
		Key:   "test_key",
		Value: []byte("blah"),
	})
	r.NoError(err)

	item, err := c.Get("test_key")
	r.NoError(err)
	r.Equal(string(item.Value), "blah")
}
