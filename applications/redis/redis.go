package redis

import (
	"context"

	"github.com/teran/go-docker-testsuite/v2"
)

type Redis interface {
	Addr() (string, error)
	MustAddr() string
	Close(ctx context.Context) error
}

type redis struct {
	c docker.Container
}

func New(ctx context.Context, image string) (Redis, error) {
	c, err := docker.
		NewContainer(
			"redis",
			image,
			nil,
			docker.NewEnvironment(),
			docker.
				NewPortBindings().
				PortDNAT(docker.ProtoTCP, 6379),
		)
	if err != nil {
		return nil, err
	}

	err = c.Run(ctx)
	if err != nil {
		return nil, err
	}

	err = c.AwaitOutput(ctx, docker.NewSubstringMatcher("* Ready to accept connections"))
	if err != nil {
		return nil, err
	}

	return &redis{
		c: c,
	}, nil
}

func (r *redis) Addr() (string, error) {
	u, err := r.c.URL(docker.ProtoTCP, 6379)
	if err != nil {
		return "", err
	}

	return u.String(), nil
}

func (r *redis) MustAddr() string {
	u, err := r.Addr()
	if err != nil {
		panic(err)
	}
	return u
}

func (r *redis) Close(ctx context.Context) error {
	return r.c.Close(ctx)
}
