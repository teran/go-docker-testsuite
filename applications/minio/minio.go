package minio

import (
	"context"
	"fmt"

	"github.com/teran/go-docker-testsuite"
	"github.com/teran/go-docker-testsuite/images"
)

const (
	tcpPortS3      = 9000
	tcpPortConsole = 9001

	MinioAccessKey       = "minioadmin"
	MinioAccessKeySecret = "minioadmin"
)

type Minio interface {
	Close(context.Context) error
	GetEndpointURL() (string, error)
	GetConsoleURL() (string, error)
}

type minio struct {
	c docker.Container
}

func New(ctx context.Context) (Minio, error) {
	return NewWithImage(ctx, images.Minio)
}

func NewWithImage(ctx context.Context, image string) (Minio, error) {
	c, err := docker.
		NewContainer(
			"minio",
			image,
			[]string{
				"server",
				"/data",
				"--address=:9000",
				"--console-address=:9001",
			},
			docker.NewEnvironment().
				StringVar("MINIO_ACCESS_KEY", MinioAccessKey).
				StringVar("MINIO_SECRET_KEY", MinioAccessKeySecret),
			docker.NewPortBindings().
				PortDNAT(docker.ProtoTCP, tcpPortS3).
				PortDNAT(docker.ProtoTCP, tcpPortConsole),
		)
	if err != nil {
		return nil, err
	}

	err = c.Run(ctx)
	if err != nil {
		return nil, err
	}

	err = c.AwaitOutput(ctx, docker.NewSubstringMatcher(
		"Warning: The standard parity is set to 0. This can lead to data loss.",
	))
	if err != nil {
		return nil, err
	}

	return &minio{
		c: c,
	}, nil
}

func (m *minio) GetEndpointURL() (string, error) {
	hp, err := m.c.URL(docker.ProtoTCP, tcpPortS3)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:%d", hp.Host, hp.Port), nil
}

func (m *minio) GetConsoleURL() (string, error) {
	hp, err := m.c.URL(docker.ProtoTCP, tcpPortS3)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:%d", hp.Host, hp.Port), nil
}

func (m *minio) Close(ctx context.Context) error {
	return m.c.Close(ctx)
}
