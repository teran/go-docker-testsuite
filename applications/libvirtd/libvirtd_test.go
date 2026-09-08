package libvirtd

import (
	"context"
	"testing"
	"time"

	"github.com/docker/docker/client"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"github.com/teran/go-docker-testsuite/images"
)

func init() {
	log.SetLevel(log.TraceLevel)
}

// requireDocker skips the test when running with -short or when the Docker
// daemon is not reachable, so this integration test degrades gracefully on
// machines without Docker instead of failing hard.
func requireDocker(t *testing.T) {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping libvirtd integration test in -short mode")
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		t.Skipf("skipping libvirtd test: unable to create docker client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := cli.Ping(ctx); err != nil {
		t.Skipf("skipping libvirtd test: docker daemon unavailable: %v", err)
	}
}

func TestLibvirtd(t *testing.T) {
	requireDocker(t)

	r := require.New(t)

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()

	app, err := NewWithImage(ctx, images.Libvirtd)
	r.NoError(err)
	r.NotNil(app)

	defer func() {
		cleanupCtx, cCancel := context.WithTimeout(context.Background(), time.Minute)
		defer cCancel()
		r.NoError(app.Close(cleanupCtx))
	}()

	// Addr must be a non-empty host:port for the libvirtd TCP endpoint.
	r.NotEmpty(app.Addr())

	// The client must be connected and report a non-zero libvirt version. This
	// does not hard-depend on /dev/kvm: without it QEMU falls back to software
	// (TCG) emulation, but libvirtd still serves its control socket.
	ver, err := app.Client().ConnectGetLibVersion()
	r.NoError(err)
	r.NotZero(ver)
}
