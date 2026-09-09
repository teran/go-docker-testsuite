package docker

import (
	"context"
	"testing"
	"time"

	dockerContainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/require"
)

const busyboxImage = "index.docker.io/library/busybox:latest"

// requireDocker skips the test when running with -short or when the Docker
// daemon is not reachable, so integration tests degrade gracefully on machines
// without Docker instead of failing hard.
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

// ---------------------------------------------------------------------------
// Unit tests: ExecResult
// ---------------------------------------------------------------------------

func TestExecResultError(t *testing.T) {
	testCases := []struct {
		name    string
		res     *ExecResult
		wantNil bool
	}{
		{
			name:    "zero exit code returns nil",
			res:     &ExecResult{Stdout: []byte("out"), Stderr: []byte("err"), ExitCode: 0},
			wantNil: true,
		},
		{
			name:    "nil receiver returns nil",
			res:     nil,
			wantNil: true,
		},
		{
			name: "non-zero exit code returns error",
			res:  &ExecResult{Stdout: []byte("out"), Stderr: []byte("boom"), ExitCode: 7},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := require.New(t)

			err := tc.res.Error()
			if tc.wantNil {
				r.NoError(err)
				return
			}

			r.Error(err)
			r.Contains(err.Error(), "7")
			r.Contains(err.Error(), "boom")
		})
	}
}

func TestExecResultCombined(t *testing.T) {
	r := require.New(t)

	res := &ExecResult{
		Stdout:   []byte("out line\n"),
		Stderr:   []byte("err line\n"),
		ExitCode: 0,
	}

	r.Equal([]byte("out line\nerr line\n"), res.Combined())
}

// ---------------------------------------------------------------------------
// Unit tests: LifecycleOption factories
// ---------------------------------------------------------------------------

func TestWithStartupCommand(t *testing.T) {
	r := require.New(t)

	c := &container{}
	WithStartupCommand("sh", "-c", "echo hi > /tmp/marker")(c)

	r.Equal([]string{"sh", "-c", "echo hi > /tmp/marker"}, c.startupCmd)
	r.Nil(c.afterReadyCmd)
	r.Nil(c.afterReadyMatcher)
}

func TestWithAfterReadyCommand(t *testing.T) {
	r := require.New(t)

	c := &container{}
	WithAfterReadyCommand(NewSubstringMatcher("READY"), "sh", "-c", "touch /tmp/marker")(c)

	r.Equal([]string{"sh", "-c", "touch /tmp/marker"}, c.afterReadyCmd)
	r.NotNil(c.afterReadyMatcher)
	r.Nil(c.startupCmd)

	// A nil matcher must be preserved as nil (the command then runs
	// immediately after start / the startup command).
	c2 := &container{}
	WithAfterReadyCommand(nil, "sh", "-c", "true")(c2)

	r.Equal([]string{"sh", "-c", "true"}, c2.afterReadyCmd)
	r.Nil(c2.afterReadyMatcher)
}

func TestWithHostConfig(t *testing.T) {
	r := require.New(t)

	c := &container{}
	WithHostConfig(WithMemoryLimit(512*1024*1024), WithCPUs(0.5))(c)

	r.Len(c.containerOpts, 2)

	// The transferred options must take effect on the HostConfig as usual.
	hc := &dockerContainer.HostConfig{}
	for _, opt := range c.containerOpts {
		opt(hc)
	}
	r.Equal(int64(512*1024*1024), hc.Memory)
	r.Equal(int64(500000000), hc.NanoCPUs)
}

// TestContainerExecNotRunning verifies that Exec fails cleanly before the
// container has been started, without requiring a Docker daemon.
func TestContainerExecNotRunning(t *testing.T) {
	r := require.New(t)

	c, err := NewContainer(
		"exec-not-running",
		busyboxImage,
		[]string{"sleep", "300"},
		NewEnvironment(),
		NewPortBindings(),
	)
	r.NoError(err)

	_, err = c.Exec(t.Context(), []string{"echo", "hello"})
	r.Error(err)
	r.Contains(err.Error(), "not running")
}

// ---------------------------------------------------------------------------
// Integration tests (require Docker)
// ---------------------------------------------------------------------------

func TestContainerExec(t *testing.T) {
	r := require.New(t)
	requireDocker(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	c, err := NewContainer(
		"exec-test",
		busyboxImage,
		[]string{"sleep", "300"},
		NewEnvironment(),
		NewPortBindings(),
	)
	r.NoError(err)
	defer func() { _ = c.Close(ctx) }()

	err = c.Run(ctx)
	r.NoError(err)

	t.Run("stdout echo exits zero", func(t *testing.T) {
		r := require.New(t)

		res, err := c.Exec(ctx, []string{"echo", "hello"})
		r.NoError(err)
		r.Equal(0, res.ExitCode)
		r.Equal("hello\n", string(res.Stdout))
		r.Empty(res.Stderr)
		r.NoError(res.Error())
	})

	t.Run("non-zero exit code is reported not errored", func(t *testing.T) {
		r := require.New(t)

		res, err := c.Exec(ctx, []string{"sh", "-c", "exit 7"})
		r.NoError(err)
		r.Equal(7, res.ExitCode)

		err = res.Error()
		r.Error(err)
		r.Contains(err.Error(), "7")
	})

	t.Run("stdout and stderr are split correctly", func(t *testing.T) {
		r := require.New(t)

		res, err := c.Exec(ctx, []string{"sh", "-c", "echo out; echo err >&2"})
		r.NoError(err)
		r.Equal(0, res.ExitCode)
		r.Equal("out\n", string(res.Stdout))
		r.Equal("err\n", string(res.Stderr))
		r.Equal([]byte("out\nerr\n"), res.Combined())
	})
}

func TestContainerWithStartupCommand(t *testing.T) {
	r := require.New(t)
	requireDocker(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	c, err := NewContainerWithLifecycle(
		"startup-cmd-test",
		busyboxImage,
		[]string{"sleep", "300"},
		NewEnvironment(),
		NewPortBindings(),
		WithStartupCommand("sh", "-c", "echo hi > /tmp/marker"),
	)
	r.NoError(err)
	defer func() { _ = c.Close(ctx) }()

	err = c.Run(ctx)
	r.NoError(err)

	// The startup command must have run during Run(), before we Exec.
	res, err := c.Exec(ctx, []string{"cat", "/tmp/marker"})
	r.NoError(err)
	r.Equal(0, res.ExitCode)
	r.Equal("hi\n", string(res.Stdout))
}

func TestContainerWithStartupCommandFailsFast(t *testing.T) {
	r := require.New(t)
	requireDocker(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	c, err := NewContainerWithLifecycle(
		"startup-cmd-fail-test",
		busyboxImage,
		[]string{"sleep", "300"},
		NewEnvironment(),
		NewPortBindings(),
		WithStartupCommand("sh", "-c", "exit 3"),
	)
	r.NoError(err)
	defer func() { _ = c.Close(ctx) }()

	// A non-zero startup exit code must fail Run() (fail-fast).
	err = c.Run(ctx)
	r.Error(err)
	r.Contains(err.Error(), "3")
}

func TestContainerWithAfterReadyCommand(t *testing.T) {
	r := require.New(t)
	requireDocker(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	c, err := NewContainerWithLifecycle(
		"after-ready-test",
		busyboxImage,
		[]string{"sh", "-c", "echo READY; sleep 300"},
		NewEnvironment(),
		NewPortBindings(),
		WithAfterReadyCommand(NewSubstringMatcher("READY"), "sh", "-c", "echo ready > /tmp/afterready"),
	)
	r.NoError(err)
	defer func() { _ = c.Close(ctx) }()

	err = c.Run(ctx)
	r.NoError(err)

	// The after-ready command runs only once the "READY" line is seen.
	res, err := c.Exec(ctx, []string{"cat", "/tmp/afterready"})
	r.NoError(err)
	r.Equal(0, res.ExitCode)
	r.Equal("ready\n", string(res.Stdout))
}

// TestContainerWithAfterReadyCommandNoMatch verifies that Run() does not hang
// forever when the readiness matcher never matches the container output — it
// returns once the context deadline elapses.
func TestContainerWithAfterReadyCommandNoMatch(t *testing.T) {
	r := require.New(t)
	requireDocker(t)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	c, err := NewContainerWithLifecycle(
		"after-ready-nomatch-test",
		busyboxImage,
		[]string{"sh", "-c", "sleep 300"}, // never prints the matcher string
		NewEnvironment(),
		NewPortBindings(),
		WithAfterReadyCommand(NewSubstringMatcher("NEVER-MATCHES"), "sh", "-c", "true"),
	)
	r.NoError(err)
	// Use a fresh context for cleanup so the expired deadline doesn't block
	// container removal.
	defer func() { _ = c.Close(context.Background()) }()

	// Run() must return (failing via the deadline) rather than hanging.
	start := time.Now()
	err = c.Run(ctx)
	r.Error(err)
	r.Less(time.Since(start), 1*time.Minute)
}

func TestNewContainerWithLifecycle(t *testing.T) {
	r := require.New(t)
	requireDocker(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	c, err := NewContainerWithLifecycle(
		"lifecycle-e2e-test",
		busyboxImage,
		[]string{"sh", "-c", "echo READY; sleep 300"},
		NewEnvironment(),
		NewPortBindings(),
		WithStartupCommand("sh", "-c", "echo hi > /tmp/startup"),
		WithAfterReadyCommand(NewSubstringMatcher("READY"), "sh", "-c", "echo done > /tmp/afterready"),
	)
	r.NoError(err)

	// t.Cleanup exercises the Close cleanup path once the test finishes.
	t.Cleanup(func() { _ = c.Close(ctx) })

	err = c.Run(ctx)
	r.NoError(err)

	// Startup command ran during Run().
	res, err := c.Exec(ctx, []string{"cat", "/tmp/startup"})
	r.NoError(err)
	r.Equal(0, res.ExitCode)
	r.Equal("hi\n", string(res.Stdout))

	// After-ready command ran once "READY" was observed.
	res, err = c.Exec(ctx, []string{"cat", "/tmp/afterready"})
	r.NoError(err)
	r.Equal(0, res.ExitCode)
	r.Equal("done\n", string(res.Stdout))
}
