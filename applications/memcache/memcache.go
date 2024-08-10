package memcache

import (
	"context"
	"fmt"
	"time"

	memcacheCli "github.com/bradfitz/gomemcache/memcache"
	log "github.com/sirupsen/logrus"

	docker "github.com/teran/go-docker-testsuite"
	"github.com/teran/go-docker-testsuite/images"
)

type Memcache interface {
	Close(context.Context) error
	GetEndpointAddress() (string, error)
}

type memcache struct {
	c docker.Container
}

func New(ctx context.Context) (Memcache, error) {
	return NewWithImage(ctx, images.Memcache)
}

func NewWithImage(ctx context.Context, image string) (Memcache, error) {
	c, err := docker.NewContainer(
		"memcache",
		image,
		[]string{},
		docker.NewEnvironment(),
		docker.NewPortBindings().
			PortDNAT(docker.ProtoTCP, 11211),
	)
	if err != nil {
		return nil, err
	}

	if err := c.Run(ctx); err != nil {
		return nil, err
	}

	hp, err := c.URL(docker.ProtoTCP, 11211)
	if err != nil {
		return nil, err
	}
	cli := memcacheCli.New(fmt.Sprintf("%s:%d", hp.Host, hp.Port))
	defer cli.Close()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Microsecond):
			if err := cli.Ping(); err != nil {
				log.Tracef("memcached is not ready yet, let's wait a bit ...")
				continue
			}

			return &memcache{
				c: c,
			}, nil
		}
	}
}

func (m *memcache) Close(ctx context.Context) error {
	return m.c.Close(ctx)
}

func (m *memcache) GetEndpointAddress() (string, error) {
	hp, err := m.c.URL(docker.ProtoTCP, 11211)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s:%d", hp.Host, hp.Port), nil
}
