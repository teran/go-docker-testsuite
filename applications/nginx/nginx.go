// Package nginx provides an nginx container for integration testing, typically
// used as a host-network reverse proxy in front of a server running on the host,
// or as a configurable web server for arbitrary routing (e.g. auth_request).
package nginx

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	docker "github.com/teran/go-docker-testsuite"
	"github.com/teran/go-docker-testsuite/images"
)

const (
	defaultBridgeListenPort = 80

	containerName     = "nginx"
	configDestination = "/etc/nginx/conf.d/default.conf"
)

// Nginx is an nginx container for integration testing. It exposes its HTTP
// listener (via Addr) and can be joined to a Group via Container.
type Nginx interface {
	// Addr returns the reachable "host:port" of the nginx HTTP listener, for
	// the test to hit. Build a URL with "http://" + Addr().
	//   - host network: returns "127.0.0.1:<listenPort>" (no Docker lookup).
	//   - bridge network: returns the host-mapped "host:port" (from URL()).
	Addr() (string, error)
	// MustAddr is Addr that panics on error.
	MustAddr() string
	// Container returns the wrapped docker.Container, for Group membership.
	Container() docker.Container
	// Close stops and removes the container.
	Close(ctx context.Context) error
}

// Option configures the nginx wrapper.
type Option func(*nginxConfig)

type nginxConfig struct {
	networkMode   docker.NetworkMode
	listenPort    uint16
	listenPortSet bool
	image         string
}

// WithNetworkMode sets the container network mode. NewWithConfig defaults to
// bridge; NewReverseProxyHost forces host (this option is ignored there).
func WithNetworkMode(mode docker.NetworkMode) Option {
	return func(c *nginxConfig) {
		c.networkMode = mode
	}
}

// WithListenPort sets the port nginx listens on. Under bridge networking this
// is the internal (unmapped) port (default 80, which is also DNAT'd); under
// host networking it is the host port nginx binds directly (default: a random
// free port allocated at construction).
func WithListenPort(port uint16) Option {
	return func(c *nginxConfig) {
		c.listenPort = port
		c.listenPortSet = true
	}
}

// WithImage overrides the nginx image (used by versioned integration tests).
func WithImage(image string) Option {
	return func(c *nginxConfig) {
		c.image = image
	}
}

type nginxImpl struct {
	c           docker.Container
	networkMode docker.NetworkMode
	listenPort  uint16
}

func defaultConfig() *nginxConfig {
	return &nginxConfig{
		networkMode: docker.NetworkModeBridge,
		listenPort:  defaultBridgeListenPort,
		image:       images.Nginx,
	}
}

// NewWithConfig starts nginx with the given config injected verbatim into
// /etc/nginx/conf.d/default.conf. config must contain one or more valid nginx
// `server { ... }` blocks. Intended for full-control cases such as auth_request
// or custom location/server routing.
//
// Network mode: bridge by default (so the wrapper can also sit in a Group and
// proxy to a sibling container by name). Pass WithNetworkMode(docker.NetworkModeHost)
// to run with host networking instead.
func NewWithConfig(ctx context.Context, config []byte, opts ...Option) (Nginx, error) {
	return newNginx(ctx, nil, config, opts...)
}

// NewWithConfigT is NewWithConfig bound to a *testing.T: the container's
// lifecycle is tied to the test and cleaned up automatically via t.Cleanup.
func NewWithConfigT(t *testing.T, ctx context.Context, config []byte, opts ...Option) (Nginx, error) {
	return newNginx(ctx, t, config, opts...)
}

func newNginx(ctx context.Context, t *testing.T, config []byte, opts ...Option) (Nginx, error) {
	cfg := defaultConfig()
	for _, o := range opts {
		o(cfg)
	}
	return startNginx(ctx, t, cfg, config)
}

// NewReverseProxyHost starts nginx with host networking to reverse-proxy every
// request to a server already listening on the host's loopback at
// 127.0.0.1:<upstreamHostPort> (the tested application, typically started in a
// goroutine in the test). nginx binds a random free host port (or the port
// given via WithListenPort) and its config is generated as:
//
//	server {
//	    listen <nginxListenPort>;
//	    location / {
//	        proxy_pass http://127.0.0.1:<upstreamHostPort>;
//	    }
//	}
//
// This is the primary use case: nginx must reach the host loopback, so it runs
// with docker.WithHostNetwork(). The returned Addr() is
// "127.0.0.1:<nginxListenPort>".
func NewReverseProxyHost(ctx context.Context, upstreamHostPort uint16, opts ...Option) (Nginx, error) {
	return newReverseProxyHost(ctx, nil, upstreamHostPort, opts...)
}

// NewReverseProxyHostT is NewReverseProxyHost bound to a *testing.T: the
// container's lifecycle is tied to the test and cleaned up automatically via
// t.Cleanup.
func NewReverseProxyHostT(t *testing.T, ctx context.Context, upstreamHostPort uint16, opts ...Option) (Nginx, error) {
	return newReverseProxyHost(ctx, t, upstreamHostPort, opts...)
}

func newReverseProxyHost(ctx context.Context, t *testing.T, upstreamHostPort uint16, opts ...Option) (Nginx, error) {
	cfg := defaultConfig()
	cfg.networkMode = docker.NetworkModeHost // forced
	for _, o := range opts {
		o(cfg)
	}
	// Host networking is hardcoded for this constructor; re-assert it after
	// applying opts so a stray WithNetworkMode cannot silently change it.
	cfg.networkMode = docker.NetworkModeHost

	if !cfg.listenPortSet {
		// Under host networking nginx binds the host port directly, so pick a
		// random free port to avoid conflicts with other host services or
		// parallel tests.
		_, port, _, err := docker.RandomPort(docker.ProtoTCP, defaultBridgeListenPort)
		if err != nil {
			return nil, errors.Wrap(err, "error allocating nginx host listen port")
		}
		cfg.listenPort = port
	}

	config := []byte(fmt.Sprintf(`server {
    listen %d;
    location / {
        proxy_pass http://127.0.0.1:%d;
    }
}
`, cfg.listenPort, upstreamHostPort))

	return startNginx(ctx, t, cfg, config)
}

// startNginx builds, runs and waits for the nginx container to be ready.
func startNginx(ctx context.Context, t *testing.T, cfg *nginxConfig, config []byte) (Nginx, error) {
	file := docker.FileFromBytes(
		configDestination, // /etc/nginx/conf.d/default.conf
		config,            // arbitrary server config
		0644,              // world-readable: nginx master runs as root, workers drop to the nginx user
		0,                 // uid: root
		0,                 // gid: root
	)

	bindings := docker.NewPortBindings()
	var hostOpts []docker.ContainerOption
	if cfg.networkMode == docker.NetworkModeHost {
		// Under host networking Docker ignores port bindings; register none.
		hostOpts = append(hostOpts, docker.WithHostNetwork())
	} else {
		bindings = bindings.PortDNAT(docker.ProtoTCP, cfg.listenPort)
	}

	lifecycleOpts := []docker.LifecycleOption{
		docker.WithFiles(file),
		docker.WithHostConfig(hostOpts...),
	}

	var (
		c   docker.Container
		err error
	)
	if t != nil {
		c, err = docker.NewContainerWithLifecycleT(t, containerName, cfg.image, nil, docker.NewEnvironment(), bindings, lifecycleOpts...)
	} else {
		c, err = docker.NewContainerWithLifecycle(containerName, cfg.image, nil, docker.NewEnvironment(), bindings, lifecycleOpts...)
	}
	if err != nil {
		return nil, errors.Wrap(err, "error creating nginx container")
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
		return nil, errors.Wrap(err, "error running nginx container")
	}

	n := &nginxImpl{
		c:           c,
		networkMode: cfg.networkMode,
		listenPort:  cfg.listenPort,
	}

	// nginx's startup log line varies across versions and, because the config
	// is arbitrary, the log surface is unpredictable. Poll the HTTP endpoint
	// instead: any response proves the listener is accepting connections.
	if err := waitForHTTPReady(ctx, n); err != nil {
		return nil, err
	}

	started = true
	return n, nil
}

// waitForHTTPReady polls Addr() until nginx accepts a connection (any valid
// HTTP response, including 4xx/5xx, proves the listener is up) or the context
// expires.
func waitForHTTPReady(ctx context.Context, n *nginxImpl) error {
	addr, err := n.Addr()
	if err != nil {
		return errors.Wrap(err, "error getting nginx address")
	}

	client := &http.Client{Timeout: 2 * time.Second}
	for {
		resp, err := client.Get("http://" + addr)
		if err == nil {
			_ = resp.Body.Close()
			log.WithFields(log.Fields{
				"status": resp.StatusCode,
				"addr":   addr,
			}).Trace("nginx HTTP endpoint is ready")
			return nil // any response ⇒ nginx is listening
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
}

func (n *nginxImpl) Addr() (string, error) {
	switch n.networkMode {
	case docker.NetworkModeHost:
		// Under host networking there are no Docker port mappings; nginx binds
		// the host port directly, so report it as 127.0.0.1:<listenPort>.
		return "127.0.0.1:" + strconv.Itoa(int(n.listenPort)), nil
	default: // bridge
		hp, err := n.c.URL(docker.ProtoTCP, n.listenPort)
		if err != nil {
			return "", err
		}
		return hp.String(), nil
	}
}

func (n *nginxImpl) MustAddr() string {
	addr, err := n.Addr()
	if err != nil {
		panic(err)
	}
	return addr
}

func (n *nginxImpl) Container() docker.Container {
	return n.c
}

func (n *nginxImpl) Close(ctx context.Context) error {
	return n.c.Close(ctx)
}
