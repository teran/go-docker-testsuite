package memcache

import (
	"context"
	"fmt"

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

	return &memcache{
		c: c,
	}, nil
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
