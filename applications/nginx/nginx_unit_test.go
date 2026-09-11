package nginx

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/require"

	docker "github.com/teran/go-docker-testsuite"
)

// fakeContainer is a minimal docker.Container used only to exercise the
// nginxImpl address logic (bridge delegation, host-net inspection, and
// error-path branches) without needing a real Docker daemon.
type fakeContainer struct {
	urlResult *docker.HostPort
	urlErr    error
}

func (f *fakeContainer) AwaitOutput(context.Context, docker.Matcher) error { return nil }
func (f *fakeContainer) Close(context.Context) error                       { return nil }
func (f *fakeContainer) Exec(context.Context, []string) (*docker.ExecResult, error) {
	return &docker.ExecResult{}, nil
}
func (f *fakeContainer) GetOutput(context.Context, ...docker.Matcher) ([]string, error) {
	return nil, nil
}
func (f *fakeContainer) ID() docker.ContainerID { return "fake" }
func (f *fakeContainer) Name() string           { return "fake" }
func (f *fakeContainer) NetworkAttach(string) error {
	return nil
}
func (f *fakeContainer) Ping(context.Context) error { return nil }
func (f *fakeContainer) Run(context.Context) error  { return nil }
func (f *fakeContainer) URL(docker.Protocol, uint16) (*docker.HostPort, error) {
	if f.urlErr != nil {
		return nil, f.urlErr
	}
	return f.urlResult, nil
}

// ---------------------------------------------------------------------------
// Inspection: host-network address logic (no Docker networking involved)
// ---------------------------------------------------------------------------

// TestHostNetworkAddrNoContainer verifies that in host-network mode Addr()
// does not consult the (nil) container at all and reports 127.0.0.1:<port>.
// This is the inspection-based equivalent of the (platform-locked) functional
// host-net reverse-proxy test: it proves the address logic for host mode
// without requiring a Linux Docker daemon.
func TestHostNetworkAddrNoContainer(t *testing.T) {
	r := require.New(t)

	n := &nginxImpl{
		c:           nil,
		networkMode: docker.NetworkModeHost,
		listenPort:  12345,
	}

	addr, err := n.Addr()
	r.NoError(err)
	r.Equal("127.0.0.1:12345", addr)
	r.Equal("127.0.0.1:12345", n.MustAddr())
}

// TestHostNetworkAddrIgnoresContainerURL verifies that even when a container
// is present, host-network Addr() never calls Container.URL() (which would
// fail for host-net containers because no port mapping exists). The fake's
// URL returns an error here; if Addr() tried to use it, the test would fail.
func TestHostNetworkAddrIgnoresContainerURL(t *testing.T) {
	r := require.New(t)

	n := &nginxImpl{
		c:           &fakeContainer{urlErr: errors.New("must not be called")},
		networkMode: docker.NetworkModeHost,
		listenPort:  9999,
	}

	addr, err := n.Addr()
	r.NoError(err)
	r.Equal("127.0.0.1:9999", addr)
	r.Equal("127.0.0.1:9999", n.MustAddr())
}

// ---------------------------------------------------------------------------
// Inspection: bridge-mode address logic delegates to Container.URL
// ---------------------------------------------------------------------------

// TestBridgeAddrDelegatesToContainerURL verifies that in bridge mode Addr()
// returns the host:port resolved via Container.URL() (using a fixed HostPort
// from the fake), and that MustAddr() equals it.
func TestBridgeAddrDelegatesToContainerURL(t *testing.T) {
	r := require.New(t)

	n := &nginxImpl{
		c: &fakeContainer{
			urlResult: &docker.HostPort{Host: "127.0.0.1", Port: 8080},
		},
		networkMode: docker.NetworkModeBridge,
		listenPort:  80,
	}

	addr, err := n.Addr()
	r.NoError(err)
	r.Equal("127.0.0.1:8080", addr)
	r.Equal("127.0.0.1:8080", n.MustAddr())
}

// ---------------------------------------------------------------------------
// Error-path branches
// ---------------------------------------------------------------------------

// TestAddrError verifies that Addr() surfaces the underlying URL() error in
// bridge mode (when the Docker port mapping cannot be resolved).
func TestAddrError(t *testing.T) {
	r := require.New(t)

	n := &nginxImpl{
		c:           &fakeContainer{urlErr: errors.New("port not mapped")},
		networkMode: docker.NetworkModeBridge,
		listenPort:  80,
	}

	_, err := n.Addr()
	r.Error(err)
	r.Contains(err.Error(), "port not mapped")
}

// TestMustAddrPanic verifies that MustAddr() panics when Addr() fails.
func TestMustAddrPanic(t *testing.T) {
	r := require.New(t)

	n := &nginxImpl{
		c:           &fakeContainer{urlErr: errors.New("port not mapped")},
		networkMode: docker.NetworkModeBridge,
		listenPort:  80,
	}

	r.Panics(func() {
		_ = n.MustAddr()
	})
}

// TestWaitForHTTPReadyTimeout verifies that waitForHTTPReady returns the
// context error when the listener never becomes reachable before the context
// is cancelled. It uses a host-network nginxImpl bound to an unused port.
func TestWaitForHTTPReadyTimeout(t *testing.T) {
	r := require.New(t)

	// Reserve a port that is (almost certainly) not listening.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	r.NoError(err)
	p := ln.Addr().(*net.TCPAddr).Port
	if p < 0 || p > 65535 {
		panic(fmt.Sprintf("port %d out of uint16 range", p))
	}
	port := uint16(p)
	r.NoError(ln.Close())
	n := &nginxImpl{
		c:           nil,
		networkMode: docker.NetworkModeHost,
		listenPort:  port,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled -> waitForHTTPReady must bail out fast

	err = waitForHTTPReady(ctx, n)
	r.Error(err)
	r.Equal(context.Canceled, err)
}
