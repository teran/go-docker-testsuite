// Package opensearch provides an OpenSearch container for integration testing.
package opensearch

import (
	"context"
	"net/http"
	"time"

	opensearchclient "github.com/opensearch-project/opensearch-go/v4"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	docker "github.com/teran/go-docker-testsuite"
	"github.com/teran/go-docker-testsuite/images"
)

const (
	httpPort = 9200
)

type OpenSearch interface {
	Addr() (string, error)
	MustAddr() string
	Client() (*opensearchclient.Client, error)
	Close(ctx context.Context) error
}

type opensearch struct {
	c docker.Container
}

func New(ctx context.Context) (OpenSearch, error) {
	return NewWithImage(ctx, images.OpenSearch)
}

func NewWithImage(ctx context.Context, image string) (OpenSearch, error) {
	c, err := docker.
		NewContainer(
			"opensearch",
			image,
			nil,
			docker.NewEnvironment().
				StringVar("discovery.type", "single-node").
				StringVar("DISABLE_SECURITY_PLUGIN", "true"),
			docker.
				NewPortBindings().
				PortDNAT(docker.ProtoTCP, httpPort),
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

	// OpenSearch's startup log line differs between versions (and the node
	// can report started before the HTTP layer is listening), so wait for the
	// HTTP endpoint to respond instead of matching a specific log message.
	if err := waitForHTTPReady(ctx, c, httpPort); err != nil {
		return nil, err
	}

	started = true
	return &opensearch{
		c: c,
	}, nil
}

// waitForHTTPReady polls the container's HTTP endpoint until it returns a
// non-5xx status or the context expires.
func waitForHTTPReady(ctx context.Context, c docker.Container, port uint16) error {
	hp, err := c.URL(docker.ProtoTCP, port)
	if err != nil {
		return errors.Wrap(err, "error getting OpenSearch URL")
	}

	client := &http.Client{Timeout: 2 * time.Second}
	for {
		resp, err := client.Get("http://" + hp.String())
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode < 500 {
				log.WithFields(log.Fields{
					"status": resp.StatusCode,
					"addr":   hp.String(),
				}).Trace("OpenSearch HTTP endpoint is ready")
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
}

func (r *opensearch) Addr() (string, error) {
	hp, err := r.c.URL(docker.ProtoTCP, httpPort)
	if err != nil {
		return "", err
	}

	return hp.String(), nil
}

func (r *opensearch) MustAddr() string {
	addr, err := r.Addr()
	if err != nil {
		panic(err)
	}
	return addr
}

func (r *opensearch) Client() (*opensearchclient.Client, error) {
	addr, err := r.Addr()
	if err != nil {
		return nil, errors.Wrap(err, "error getting OpenSearch address")
	}

	client, err := opensearchclient.NewClient(opensearchclient.Config{
		Addresses: []string{"http://" + addr},
	})
	if err != nil {
		return nil, errors.Wrap(err, "error creating OpenSearch client")
	}

	return client, nil
}

func (r *opensearch) Close(ctx context.Context) error {
	return r.c.Close(ctx)
}
