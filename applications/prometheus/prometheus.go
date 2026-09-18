// Package prometheus provides a typed wrapper around a Prometheus container
// for integration testing.
//
// It manages the lifecycle of a single `prom/prometheus` container (via the
// official Docker SDK), waits for it to become ready (HTTP GET on the API
// endpoint), and exposes a typed client interface backed by the Prometheus
// HTTP API (github.com/prometheus/client_golang/api/prometheus/v1).
package prometheus

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"

	"github.com/pkg/errors"
	docker "github.com/teran/go-docker-testsuite"
	"github.com/teran/go-docker-testsuite/images"
	wait "github.com/teran/go-docker-testsuite/wait"
)

// Prometheus is the typed client interface exposed by the Prometheus wrapper.
type Prometheus interface {
	// Client returns the Prometheus HTTP API v1 client.
	Client() promv1.API
	// Close shuts down the Prometheus container.
	Close(ctx context.Context) error
}

type prometheus struct {
	c docker.Container

	api promv1.API
}

// New starts a Prometheus container using the default image.
func New(ctx context.Context) (Prometheus, error) {
	return NewWithImage(ctx, images.Prometheus)
}

// NewWithImage starts a Prometheus container using the given image.
func NewWithImage(ctx context.Context, image string) (Prometheus, error) {
	c, err := docker.
		NewContainer(
			"prometheus",
			image,
			nil,
			docker.NewEnvironment(),
			docker.
				NewPortBindings().
				PortDNAT(docker.ProtoTCP, 9090),
		)
	if err != nil {
		return nil, errors.Wrap(err, "error creating new container")
	}

	app := &prometheus{
		c: c,
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
		return nil, errors.Wrap(err, "error running container")
	}

	// Prometheus accepts TCP connections before its HTTP server is fully
	// ready, and the ForHTTPGet probe treats that early EOF as fatal. Probe
	// readiness by running wget against the /-/ready endpoint inside the
	// container instead: it only returns success once the server responds 200.
	if err := wait.Wait(ctx, c, wait.ForCommand([]string{"wget", "-q", "-O", "/dev/null", "http://127.0.0.1:9090/-/ready"})); err != nil {
		return nil, errors.Wrap(err, "error waiting for Prometheus to become ready")
	}

	hp, err := c.URL(docker.ProtoTCP, 9090)
	if err != nil {
		return nil, errors.Wrap(err, "error getting container URL")
	}

	promAPI, err := api.NewClient(api.Config{
		Address: "http://" + hp.String(),
	})
	if err != nil {
		return nil, errors.Wrap(err, "error creating Prometheus client")
	}
	app.api = promv1.NewAPI(promAPI)

	started = true
	return app, nil
}

// NewWithT is New bound to a *testing.T: the container's lifecycle is tied to
// the test and cleaned up automatically via t.Cleanup.
func NewWithT(t *testing.T, ctx context.Context) (Prometheus, error) {
	return NewWithImageT(t, ctx, images.Prometheus)
}

// NewWithImageT is NewWithImage bound to a *testing.T: the container's
// lifecycle is tied to the test and cleaned up automatically via t.Cleanup.
func NewWithImageT(t *testing.T, ctx context.Context, image string) (Prometheus, error) {
	c, err := docker.
		NewContainerWithT(
			t,
			"prometheus",
			image,
			nil,
			docker.NewEnvironment(),
			docker.
				NewPortBindings().
				PortDNAT(docker.ProtoTCP, 9090),
		)
	if err != nil {
		return nil, errors.Wrap(err, "error creating new container")
	}

	app := &prometheus{
		c: c,
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
		return nil, errors.Wrap(err, "error running container")
	}

	if err := wait.Wait(ctx, c, wait.ForCommand([]string{"wget", "-q", "-O", "/dev/null", "http://127.0.0.1:9090/-/ready"})); err != nil {
		return nil, errors.Wrap(err, "error waiting for Prometheus to become ready")
	}

	hp, err := c.URL(docker.ProtoTCP, 9090)
	if err != nil {
		return nil, errors.Wrap(err, "error getting container URL")
	}

	promAPI, err := api.NewClient(api.Config{
		Address: "http://" + hp.String(),
	})
	if err != nil {
		return nil, errors.Wrap(err, "error creating Prometheus client")
	}
	app.api = promv1.NewAPI(promAPI)

	started = true
	return app, nil
}

func (p *prometheus) Client() promv1.API {
	return p.api
}

func (p *prometheus) Close(ctx context.Context) error {
	return errors.Wrap(p.c.Close(ctx), "error closing Prometheus")
}
