// Package ceph provides a Ceph RGW (S3) container for integration testing.
package ceph

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	docker "github.com/teran/go-docker-testsuite"
	"github.com/teran/go-docker-testsuite/images"
)

const (
	tcpPortRGW = 8080
	tcpPortMON = 3300

	// DefaultUID is the RGW admin user created by the demo image.
	DefaultUID = "demo"
	// DefaultAccessKey / DefaultSecretKey are the demo RGW credentials
	// injected via CEPH_DEMO_ACCESS_KEY / CEPH_DEMO_SECRET_KEY.
	DefaultAccessKey = "access"
	DefaultSecretKey = "secret"
)

// Ceph is a running single-container Ceph cluster (mon, mgr, osd, rgw).
type Ceph interface {
	Close(ctx context.Context) error
	// Endpoint returns the RGW S3 endpoint as host:port (no scheme).
	Endpoint() (string, error)
	AccessKey() string
	SecretKey() string
	// Client returns an S3 client (AWS SDK v2) configured for the RGW.
	Client() (*s3.Client, error)
}

type ceph struct {
	c docker.Container

	accessKey string
	secretKey string
}

// New starts a Ceph demo container with the default image.
func New(ctx context.Context) (Ceph, error) {
	return NewWithImage(ctx, images.Ceph)
}

// NewWithImage starts a Ceph demo container with a custom image. Images are
// published multi-arch (amd64+arm64) as ghcr.io/teran/ceph-container/ceph:v<version>
// from github.com/teran/ceph-container (squid and tentacle release trains).
func NewWithImage(ctx context.Context, image string) (Ceph, error) {
	c, err := docker.
		NewContainer(
			"ceph",
			image,
			nil,
			docker.
				NewEnvironment().
				StringVar("MON_IP", "127.0.0.1").
				StringVar("CEPH_PUBLIC_NETWORK", "0.0.0.0/0").
				StringVar("CEPH_DEMO_UID", DefaultUID).
				StringVar("CEPH_DEMO_ACCESS_KEY", DefaultAccessKey).
				StringVar("CEPH_DEMO_SECRET_KEY", DefaultSecretKey).
				StringVar("RGW_NAME", "localhost"),
			docker.
				NewPortBindings().
				PortDNAT(docker.ProtoTCP, tcpPortRGW).
				PortDNAT(docker.ProtoTCP, tcpPortMON),
			docker.WithUlimit("nofile", 65536, 65536),
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

	if err := c.Run(ctx); err != nil {
		return nil, err
	}

	// The image logs "SUCCESS: RGW on ..." once the RGW is up and the admin
	// user is created.
	if err := c.AwaitOutput(ctx, docker.NewSubstringMatcher("SUCCESS")); err != nil {
		return nil, err
	}

	started = true
	return &ceph{
		c:         c,
		accessKey: DefaultAccessKey,
		secretKey: DefaultSecretKey,
	}, nil
}

func (c *ceph) Endpoint() (string, error) {
	hp, err := c.c.URL(docker.ProtoTCP, tcpPortRGW)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:%d", hp.Host, hp.Port), nil
}

func (c *ceph) AccessKey() string {
	return c.accessKey
}

func (c *ceph) SecretKey() string {
	return c.secretKey
}

func (c *ceph) Client() (*s3.Client, error) {
	endpoint, err := c.Endpoint()
	if err != nil {
		return nil, err
	}

	cli := s3.New(s3.Options{
		Region:       "us-east-1",
		BaseEndpoint: aws.String("http://" + endpoint),
		Credentials: aws.NewCredentialsCache(
			credentials.NewStaticCredentialsProvider(c.accessKey, c.secretKey, ""),
		),
		// RGW uses path-style addressing (no virtual-hosted buckets).
		UsePathStyle: true,
	})

	return cli, nil
}

func (c *ceph) Close(ctx context.Context) error {
	return c.c.Close(ctx)
}
