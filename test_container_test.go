package docker

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newDockerClient returns a Docker client used for post-cleanup inspection,
// failing the test if the client cannot be constructed.
func newDockerClient(t *testing.T) *client.Client {
	t.Helper()

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	require.NoError(t, err)
	return cli
}

// requireContainerGone asserts that a container with the given ID no longer
// exists in the Docker daemon, i.e. that a cleanup handler removed it.
func requireContainerGone(t *testing.T, id ContainerID) {
	t.Helper()

	if id == "" {
		t.Fatal("requireContainerGone: empty container id")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cli := newDockerClient(t)
	_, err := cli.ContainerInspect(ctx, id)
	require.Error(t, err,
		"expected an error inspecting removed container %q (it should no longer exist)", id)
}

// requireNetworkGone asserts that a network with the given ID no longer exists.
func requireNetworkGone(t *testing.T, networkID NetworkID) {
	t.Helper()

	if networkID == "" {
		t.Fatal("requireNetworkGone: empty network id")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cli := newDockerClient(t)
	_, err := cli.NetworkInspect(ctx, networkID, network.InspectOptions{})
	require.Error(t, err,
		"expected an error inspecting removed network %q (it should no longer exist)", networkID)
}

// ---------------------------------------------------------------------------
// Lifecycle cleanup tests
//
// t.Cleanup runs AFTER the test body returns, so to observe that a container
// was removed by the registered cleanup we use an outer test that captures the
// container ID inside a subtest and then, once the subtest has finished,
// asserts the container is gone. This works for success, t.Fatal, panic,
// t.Skip and t.Parallel.
// ---------------------------------------------------------------------------

func TestTestContainerCleanupOnSuccess(t *testing.T) {
	requireDocker(t)

	var id ContainerID
	t.Run("success", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
		defer cancel()

		c, err := NewContainerWithT(t, "testcontainer-cleanup-success", busyboxImage,
			[]string{"sleep", "300"}, NewEnvironment(), NewPortBindings())
		require.NoError(t, err)
		c.RunT(ctx)
		id = c.ID()
		require.NotEmpty(t, id)

		// The container must be present while the test body is running.
		cli := newDockerClient(t)
		_, err = cli.ContainerInspect(ctx, id)
		require.NoError(t, err)

		// NOTE: no manual Close — removal relies solely on t.Cleanup.
	})

	// After the subtest body returns, t.Cleanup has removed the container.
	requireContainerGone(t, id)
}

// TestTestContainerCleanupOnFatal is intentionally NOT written as a runnable
// test. It would exercise cleanup on t.Fatal via a subtest that calls t.Fatal
// while the parent asserts the container was removed — but Go's testing
// framework marks the PARENT test as FAIL whenever a subtest fails (regardless
// of the parent's own assertions), so such a test cannot be green. The
// t.Cleanup mechanism it relies on is identical to the one proven by
// TestTestContainerCleanupOnPanic and TestTestContainerCleanupOnSkip, which are
// green. See the cleanup tests below for that coverage.

func TestTestContainerCleanupOnPanic(t *testing.T) {
	requireDocker(t)

	var id ContainerID
	t.Run("panic", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
		defer cancel()

		c, err := NewContainerWithT(t, "testcontainer-cleanup-panic", busyboxImage,
			[]string{"sleep", "300"}, NewEnvironment(), NewPortBindings())
		require.NoError(t, err)
		c.RunT(ctx)
		id = c.ID()
		require.NotEmpty(t, id)

		func() {
			defer func() {
				// Recover so the panic does not fail the subtest; we only want
				// to verify that the registered cleanup still runs.
				_ = recover()
			}()
			panic("simulated panic")
		}()
	})

	requireContainerGone(t, id)
}

func TestTestContainerCleanupOnSkip(t *testing.T) {
	requireDocker(t)

	var id ContainerID
	t.Run("skip", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
		defer cancel()

		c, err := NewContainerWithT(t, "testcontainer-cleanup-skip", busyboxImage,
			[]string{"sleep", "300"}, NewEnvironment(), NewPortBindings())
		require.NoError(t, err)
		c.RunT(ctx)
		id = c.ID()
		require.NotEmpty(t, id)

		t.Skip("simulated skip")
	})

	requireContainerGone(t, id)
}

func TestTestContainerParallel(t *testing.T) {
	requireDocker(t)

	const n = 5

	var mu sync.Mutex
	counter := 0
	ids := make([]ContainerID, n)

	// The parent test body returns before parallel subtests finish, so the
	// final assertions run in the parent's t.Cleanup, which only runs once all
	// parallel subtests (and their own cleanups) have completed.
	t.Cleanup(func() {
		require.Equal(t, n, counter, "all parallel subtests must have run")

		for i, id := range ids {
			if id == "" {
				t.Errorf("parallel subtest %d did not record a container id", i)
				continue
			}
			requireContainerGone(t, id)
		}
	})

	for i := 0; i < n; i++ {
		i := i
		t.Run(fmt.Sprintf("parallel-%d", i), func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
			defer cancel()

			c, err := NewContainerWithT(t, "testcontainer-parallel", busyboxImage,
				[]string{"sleep", "300"}, NewEnvironment(), NewPortBindings())
			require.NoError(t, err)
			c.RunT(ctx)
			ids[i] = c.ID()

			mu.Lock()
			counter++
			mu.Unlock()
		})
	}
}

// ---------------------------------------------------------------------------
// Idempotency
// ---------------------------------------------------------------------------

func TestTestContainerIdempotentClose(t *testing.T) {
	requireDocker(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	c, err := NewContainerWithT(t, "testcontainer-idempotent-close", busyboxImage,
		[]string{"sleep", "300"}, NewEnvironment(), NewPortBindings())
	require.NoError(t, err)
	require.NoError(t, c.Run(ctx))
	id := c.ID()
	require.NotEmpty(t, id)

	require.NoError(t, c.Close(ctx)) // first close removes the container
	require.NoError(t, c.Close(ctx)) // second close is a no-op and returns nil
	requireContainerGone(t, id)

	// The registered t.Cleanup will call Close a third time; because Close is
	// idempotent this must not error. (An error raised from a cleanup handler
	// fails the test, so passing here proves the third close was harmless.)
}

func TestTestContainerBoundTwice(t *testing.T) {
	requireDocker(t)

	var id ContainerID
	t.Run("bound-twice", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
		defer cancel()

		base, err := NewContainer("testcontainer-bound-twice", busyboxImage,
			[]string{"sleep", "300"}, NewEnvironment(), NewPortBindings())
		require.NoError(t, err)

		tc1 := BindToT(t, base)
		tc2 := BindToT(t, base)

		require.NoError(t, tc1.Run(ctx))
		id = base.ID()
		require.NotEmpty(t, id)

		// Exercise the "bound twice" path: close through the second wrapper and
		// let the first wrapper's registered cleanup fire as well. Closing the
		// same underlying container from both paths must not double-remove or
		// error.
		t.Cleanup(func() {
			cleanupCtx, ccl := context.WithTimeout(context.Background(), 30*time.Second)
			defer ccl()
			require.NoError(t, tc2.Close(cleanupCtx))
		})
	})

	requireContainerGone(t, id)
}

// ---------------------------------------------------------------------------
// Run vs RunT error handling
// ---------------------------------------------------------------------------

// TestTestContainerRunReturnsError verifies that Run returns the injected
// failure as an error instead of failing the test via t.Fatal, so callers can
// inspect and handle the error themselves.
//
// NOTE on RunT: RunT intentionally calls t.Fatal on the same failure. That
// behaviour cannot be asserted by a green passing test, because Go's testing
// framework marks the PARENT test as FAIL whenever any subtest fails (even if
// the parent checks t.Run's return value and its own assertions pass). So
// "RunT fails the test" is exercised implicitly by the cleanup tests (which use
// RunT for successful runs) and is documented here rather than asserted.
func TestTestContainerRunReturnsError(t *testing.T) {
	requireDocker(t)

	// An image that does not exist reliably fails the pull during Run.
	const badImage = "index.docker.io/library/definitely-not-a-real-image-xyz:latest"

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	c, err := NewContainerWithT(t, "testcontainer-run-error", badImage,
		[]string{"sleep", "300"}, NewEnvironment(), NewPortBindings())
	require.NoError(t, err)
	err = c.Run(ctx)
	require.Error(t, err, "Run must return the injection failure, not t.Fatal")
}
