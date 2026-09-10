package docker

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestContainer binds a Container to a *testing.T so its lifecycle is tied to
// the test. It implements the full Container interface by delegating to the
// wrapped Container, while additionally:
//
//   - registering a t.Cleanup handler on the first Run so the container is
//     always stopped and removed when the test finishes (including on success,
//     t.Fatal, panic and t.Skip), and
//   - making Close idempotent, so overlapping cleanups (the t.Cleanup handler
//     and an explicit wrapper Close) do not double-remove the container.
//
// Each binding owns its own lifecycle, so TestContainer is safe to use with
// t.Parallel(): cleanups are registered against the correct per-test T and run
// in LIFO order within that test's context.
type TestContainer struct {
	t         *testing.T
	c         Container
	image     string
	runOnce   sync.Once
	closeOnce sync.Once
}

// BindToT wraps an existing Container with a *testing.T, tying its lifecycle
// (run cleanup + idempotent close) to the test. Use this when the container was
// created with the base NewContainer/NewContainerWithLifecycle constructors but
// should be cleaned up automatically by the test.
func BindToT(t *testing.T, c Container) *TestContainer {
	return &TestContainer{t: t, c: c}
}

// NewContainerWithT is NewContainer bound to a *testing.T. It creates a
// container exactly like NewContainer but returns a TestContainer whose
// lifecycle is tied to the test.
func NewContainerWithT(t *testing.T, name, image string, cmd []string, env Environment, ports *PortBindings, opts ...ContainerOption) (*TestContainer, error) {
	c, err := NewContainer(name, image, cmd, env, ports, opts...)
	if err != nil {
		return nil, err
	}
	return &TestContainer{t: t, c: c, image: image}, nil
}

// NewContainerWithLifecycleT is NewContainerWithLifecycle bound to a
// *testing.T. It behaves like NewContainerWithLifecycle but returns a
// TestContainer whose lifecycle is tied to the test.
func NewContainerWithLifecycleT(t *testing.T, name, image string, cmd []string, env Environment, ports *PortBindings, opts ...LifecycleOption) (*TestContainer, error) {
	c, err := NewContainerWithLifecycle(name, image, cmd, env, ports, opts...)
	if err != nil {
		return nil, err
	}
	return &TestContainer{t: t, c: c, image: image}, nil
}

// describe returns a human-readable identifier for log messages, including the
// image when it is known (i.e. when created via the NewContainer*T
// constructors; BindToT does not carry image metadata).
func (tc *TestContainer) describe() string {
	if tc.image == "" {
		return tc.Name()
	}
	return fmt.Sprintf("%s (image %s)", tc.Name(), tc.image)
}

// logf logs through the bound testing.T, attributing the message to the test
// function that triggered it (rather than to the TestContainer internals).
func (tc *TestContainer) logf(format string, args ...interface{}) {
	tc.t.Helper()
	tc.t.Logf(format, args...)
}

// registerCleanup registers the t.Cleanup handler exactly once, on the first
// Run. The handler closes the container with a 30-second timeout; Close is
// idempotent so an earlier or concurrent explicit Close is harmless. t.Cleanup
// runs on test success, t.Fatal, panic and t.Skip, in LIFO order.
func (tc *TestContainer) registerCleanup() {
	tc.runOnce.Do(func() {
		tc.t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_ = tc.Close(ctx)
		})
	})
}

// Run starts the container and registers the test cleanup on the first call.
// On error it returns the error (logging it via t.Logf) rather than failing the
// test, so it remains compatible with applications and groups.
func (tc *TestContainer) Run(ctx context.Context) error {
	tc.registerCleanup()
	tc.logf("container %s run started", tc.describe())

	err := tc.c.Run(ctx)
	if err != nil {
		tc.logf("container %s run failed: %v", tc.describe(), err)
		return err
	}

	tc.logf("container %s run completed", tc.describe())
	return nil
}

// RunT starts the container, registers the test cleanup and fails the test
// immediately (t.Fatal) if the container fails to start.
func (tc *TestContainer) RunT(ctx context.Context) {
	tc.registerCleanup()
	tc.logf("container %s run started", tc.describe())

	if err := tc.c.Run(ctx); err != nil {
		tc.logf("container %s run failed: %v", tc.describe(), err)
		tc.t.Fatal(err)
	}

	tc.logf("container %s run completed", tc.describe())
}

// Close stops and removes the container. It is idempotent: only the first call
// performs the work; subsequent calls return nil.
func (tc *TestContainer) Close(ctx context.Context) error {
	var err error
	tc.closeOnce.Do(func() {
		tc.logf("container %s close started", tc.describe())
		if err = tc.c.Close(ctx); err != nil {
			tc.logf("container %s close failed: %v", tc.describe(), err)
		} else {
			tc.logf("container %s closed", tc.describe())
		}
	})
	return err
}

// AwaitOutput delegates to the wrapped container.
func (tc *TestContainer) AwaitOutput(ctx context.Context, m Matcher) error {
	return tc.c.AwaitOutput(ctx, m)
}

// Exec delegates to the wrapped container, logging the outcome at a low-noise
// level.
func (tc *TestContainer) Exec(ctx context.Context, cmd []string) (*ExecResult, error) {
	res, err := tc.c.Exec(ctx, cmd)
	if err != nil {
		tc.logf("container %s exec failed: %v", tc.describe(), err)
	}
	return res, err
}

// GetOutput delegates to the wrapped container.
func (tc *TestContainer) GetOutput(ctx context.Context, ms ...Matcher) ([]string, error) {
	return tc.c.GetOutput(ctx, ms...)
}

// ID returns the Docker container ID (empty until Run).
func (tc *TestContainer) ID() ContainerID {
	return tc.c.ID()
}

// Name returns the container name.
func (tc *TestContainer) Name() string {
	return tc.c.Name()
}

// NetworkAttach delegates to the wrapped container.
func (tc *TestContainer) NetworkAttach(networkID string) error {
	return tc.c.NetworkAttach(networkID)
}

// Ping delegates to the wrapped container, logging the outcome at a low-noise
// level.
func (tc *TestContainer) Ping(ctx context.Context) error {
	err := tc.c.Ping(ctx)
	if err != nil {
		tc.logf("container %s ping failed: %v", tc.describe(), err)
	}
	return err
}

// URL delegates to the wrapped container.
func (tc *TestContainer) URL(proto Protocol, port uint16) (*HostPort, error) {
	return tc.c.URL(proto, port)
}
