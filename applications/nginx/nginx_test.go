package nginx_test

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/docker/docker/client"
	"github.com/stretchr/testify/require"

	docker "github.com/teran/go-docker-testsuite"
	"github.com/teran/go-docker-testsuite/applications/nginx"
)

// requireDocker skips the test when Docker is unavailable or -short is set, so
// the integration tests degrade gracefully on machines without Docker instead
// of failing hard.
func requireDocker(t *testing.T) {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		t.Skipf("skipping integration test: unable to create docker client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := cli.Ping(ctx); err != nil {
		t.Skipf("skipping integration test: docker daemon unavailable: %v", err)
	}
}

// requireContainerGone asserts that a container with the given ID was removed.
func requireContainerGone(t *testing.T, id docker.ContainerID) {
	t.Helper()

	if id == "" {
		t.Fatal("requireContainerGone: empty container id")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	require.NoError(t, err)

	_, err = cli.ContainerInspect(ctx, id)
	require.Error(t, err,
		"expected an error inspecting removed container %q (it should no longer exist)", id)
}

// getBody performs a GET against url and returns the response body.
func getBody(t *testing.T, url string) (int, string) {
	t.Helper()

	cli := &http.Client{Timeout: 5 * time.Second}
	resp, err := cli.Get(url)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp.StatusCode, string(data)
}

// ---------------------------------------------------------------------------
// Generic config injection (bridge default)
// ---------------------------------------------------------------------------

func TestNewWithConfig(t *testing.T) {
	requireDocker(t)

	config := []byte(`server {
    listen 80;
    location / {
        return 200 "custom";
    }
}
`)

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()

	app, err := nginx.NewWithConfig(ctx, config)
	require.NoError(t, err)
	defer func() { _ = app.Close(ctx) }()

	addr := app.MustAddr()

	code, body := getBody(t, "http://"+addr+"/")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, "custom", body)
}

// ---------------------------------------------------------------------------
// Readiness: nginx is "ready" once it is listening, even against a bogus
// upstream (it returns 502 rather than failing to dial).
// ---------------------------------------------------------------------------

func TestReadinessAnyResponse(t *testing.T) {
	requireDocker(t)

	// Point nginx at an upstream that is not listening. nginx itself should
	// still start and listen, returning 502 on requests.
	config := []byte(`server {
    listen 80;
    location / {
        proxy_pass http://127.0.0.1:9999;
    }
}
`)

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()

	// The constructor's readiness check only requires *any* HTTP response, so
	// it must return successfully even though the upstream is dead.
	app, err := nginx.NewWithConfig(ctx, config)
	require.NoError(t, err)
	defer func() { _ = app.Close(ctx) }()

	addr := app.MustAddr()

	code, _ := getBody(t, "http://"+addr+"/")
	require.Equal(t, http.StatusBadGateway, code)
}

// ---------------------------------------------------------------------------
// Accessors: Addr / MustAddr (bridge network mode)
// ---------------------------------------------------------------------------

func TestAddrMustAddr(t *testing.T) {
	requireDocker(t)

	config := []byte(`server {
    listen 80;
    location / {
        return 200 "bridge";
    }
}
`)

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()

	app, err := nginx.NewWithConfig(ctx, config)
	require.NoError(t, err)
	defer func() { _ = app.Close(ctx) }()

	// Addr() is dialable, MustAddr() equals Addr(), and MustAddr() does not
	// panic on the happy path.
	addr, err := app.Addr()
	require.NoError(t, err)
	require.NotEmpty(t, addr)
	require.Equal(t, addr, app.MustAddr())

	code, body := getBody(t, "http://"+addr+"/")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, "bridge", body)
}

// ---------------------------------------------------------------------------
// *T variants: lifecycle tied to the test, auto-removal via t.Cleanup
// ---------------------------------------------------------------------------

func TestNewWithConfigT(t *testing.T) {
	requireDocker(t)

	config := []byte(`server {
    listen 80;
    location / {
        return 200 "t-variant";
    }
}
`)

	var cid docker.ContainerID
	t.Run("new-with-config-t", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
		defer cancel()

		app, err := nginx.NewWithConfigT(t, ctx, config)
		require.NoError(t, err)
		require.NotNil(t, app)

		cid = app.Container().ID()
		require.NotEmpty(t, cid)

		// No manual Close: the container lifecycle is tied to the test.
		code, body := getBody(t, "http://"+app.MustAddr()+"/")
		require.Equal(t, http.StatusOK, code)
		require.Equal(t, "t-variant", body)
	})

	// After the subtest, the registered t.Cleanup removed the container.
	requireContainerGone(t, cid)
}
