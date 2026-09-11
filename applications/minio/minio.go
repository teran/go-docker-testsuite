package minio

import (
	"context"
	"fmt"
	"testing"
	"time"

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
				StringVar("MINIO_ROOT_USER", MinioAccessKey).
				StringVar("MINIO_ROOT_PASSWORD", MinioAccessKeySecret),
			docker.NewPortBindings().
				PortDNAT(docker.ProtoTCP, tcpPortS3).
				PortDNAT(docker.ProtoTCP, tcpPortConsole),
		)
	if err != nil {
		return nil, err
	}

	started := false
	defer func() {
		if !started {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_ = c.Close(cleanupCtx)
		}
	}()

	err = c.Run(ctx)
	if err != nil {
		return nil, err
	}

	err = c.AwaitOutput(ctx, docker.NewSubstringMatcher(
		"API: http://",
	))
	if err != nil {
		return nil, err
	}

	started = true
	return &minio{
		c: c,
	}, nil
}

// NewWithT is New bound to a *testing.T: the container's lifecycle is tied to
// the test and cleaned up automatically via t.Cleanup.
func NewWithT(t *testing.T, ctx context.Context) (Minio, error) {
	return NewWithImageT(t, ctx, images.Minio)
}

// NewWithImageT is NewWithImage bound to a *testing.T: the container's
// lifecycle is tied to the test and cleaned up automatically via t.Cleanup.
func NewWithImageT(t *testing.T, ctx context.Context, image string) (Minio, error) {
	c, err := docker.
		NewContainerWithT(
			t,
			"minio",
			image,
			[]string{
				"server",
				"/data",
				"--address=:9000",
				"--console-address=:9001",
			},
			docker.NewEnvironment().
				StringVar("MINIO_ROOT_USER", MinioAccessKey).
				StringVar("MINIO_ROOT_PASSWORD", MinioAccessKeySecret),
			docker.NewPortBindings().
				PortDNAT(docker.ProtoTCP, tcpPortS3).
				PortDNAT(docker.ProtoTCP, tcpPortConsole),
		)
	if err != nil {
		return nil, err
	}

	started := false
	defer func() {
		if !started {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_ = c.Close(cleanupCtx)
		}
	}()

	err = c.Run(ctx)
	if err != nil {
		return nil, err
	}

	err = c.AwaitOutput(ctx, docker.NewSubstringMatcher(
		"API: http://",
	))
	if err != nil {
		return nil, err
	}

	started = true
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
	hp, err := m.c.URL(docker.ProtoTCP, tcpPortConsole)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:%d", hp.Host, hp.Port), nil
}

func (m *minio) Close(ctx context.Context) error {
	return m.c.Close(ctx)
}
