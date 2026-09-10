package docker

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestGroup binds a Group to a *testing.T so its lifecycle is tied to the test.
// It delegates Run/Close to the wrapped Group while additionally registering a
// t.Cleanup handler on the first Run and making Close idempotent.
//
// The group's Close removes the member containers and then the internal
// network. This is protected against double-removal both by the idempotent
// group Close and by the idempotent per-container Close (so a group whose
// members are individually bound to the test is still safe to clean up from
// both paths). Like TestContainer, TestGroup is safe to use with t.Parallel().
type TestGroup struct {
	t         *testing.T
	g         Group
	runOnce   sync.Once
	closeOnce sync.Once
}

// BindGroupToT wraps an existing Group with a *testing.T, tying its lifecycle
// (run cleanup + idempotent close) to the test. Use this when the group was
// created with the base NewGroup constructor but should be cleaned up
// automatically by the test.
func BindGroupToT(t *testing.T, g Group) *TestGroup {
	return &TestGroup{t: t, g: g}
}

// NewGroupT is NewGroup bound to a *testing.T. It creates a group exactly like
// NewGroup but returns a TestGroup whose lifecycle is tied to the test.
func NewGroupT(t *testing.T, name string, apps ...*Application) (*TestGroup, error) {
	g, err := NewGroup(name, apps...)
	if err != nil {
		return nil, err
	}
	return BindGroupToT(t, g), nil
}

// logf logs through the bound testing.T, attributing the message to the test
// function that triggered it (rather than to the TestGroup internals).
func (tg *TestGroup) logf(format string, args ...interface{}) {
	tg.t.Helper()
	tg.t.Logf(format, args...)
}

// registerCleanup registers the t.Cleanup handler exactly once, on the first
// Run. The handler closes the group with a 30-second timeout; Close is
// idempotent so an earlier or concurrent explicit Close is harmless. t.Cleanup
// runs on test success, t.Fatal, panic and t.Skip, in LIFO order.
func (tg *TestGroup) registerCleanup() {
	tg.runOnce.Do(func() {
		tg.t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_ = tg.Close(ctx)
		})
	})
}

// Run starts the group and registers the test cleanup on the first call. On
// error it returns the error (logging it via t.Logf) rather than failing the
// test.
func (tg *TestGroup) Run(ctx context.Context) error {
	tg.registerCleanup()
	tg.logf("group run started")

	if err := tg.g.Run(ctx); err != nil {
		tg.logf("group run failed: %v", err)
		return err
	}

	tg.logf("group run completed")
	return nil
}

// RunT starts the group, registers the test cleanup and fails the test
// immediately (t.Fatal) if the group fails to start.
func (tg *TestGroup) RunT(ctx context.Context) {
	tg.registerCleanup()
	tg.logf("group run started")

	if err := tg.g.Run(ctx); err != nil {
		tg.logf("group run failed: %v", err)
		tg.t.Fatal(err)
	}

	tg.logf("group run completed")
}

// Close removes the group's member containers and internal network. It is
// idempotent: only the first call performs the work; subsequent calls return
// nil.
func (tg *TestGroup) Close(ctx context.Context) error {
	var err error
	tg.closeOnce.Do(func() {
		tg.logf("group close started")
		if err = tg.g.Close(ctx); err != nil {
			tg.logf("group close failed: %v", err)
		} else {
			tg.logf("group closed")
		}
	})
	return err
}
