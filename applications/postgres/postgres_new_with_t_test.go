package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/docker/docker/client"
	pgx "github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	docker "github.com/teran/go-docker-testsuite"
)

// requireDocker skips the test when Docker is unavailable or -short is set.
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

// TestNewWithT verifies the Application.NewWithT binding: the DB container's
// lifecycle is tied to the test and removed automatically via t.Cleanup even
// without any manual Close. We use the subtest pattern because t.Cleanup runs
// after the test body, so the parent asserts removal once the subtest ends.
func TestNewWithT(t *testing.T) {
	requireDocker(t)

	var cid docker.ContainerID
	t.Run("new-with-t", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
		defer cancel()

		pg, err := NewWithT(t, ctx)
		require.NoError(t, err)
		require.NotNil(t, pg)

		// Capture the underlying container id to assert removal afterwards.
		cid = pg.(*postgresql).c.ID()
		require.NotEmpty(t, cid)

		// Connect and run a query; no manual Close is performed.
		dsn := pg.MustDSN("postgres")
		conn, err := pgx.Connect(ctx, dsn)
		require.NoError(t, err)

		var v int
		require.NoError(t, conn.QueryRow(ctx, "SELECT 42").Scan(&v))
		require.Equal(t, 42, v)
		require.NoError(t, conn.Close(ctx))
	})

	// After the subtest, the registered t.Cleanup removed the DB container.
	requireContainerGone(t, cid)
}
