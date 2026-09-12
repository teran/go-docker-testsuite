package wait

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWait_ReadyImmediately(t *testing.T) {
	r := require.New(t)

	var calls int
	strategy := func(ctx context.Context, t Target) (bool, error) {
		calls++
		return true, nil
	}

	start := time.Now()
	err := Wait(context.Background(), newStubTarget(), strategy,
		WithInterval(50*time.Millisecond), WithTimeout(5*time.Second))
	r.NoError(err)
	r.Equal(1, calls)
	// Ready on the first poll, well before a single interval elapses.
	r.Less(time.Since(start), 200*time.Millisecond)
}

func TestWait_RetriesUntilReady(t *testing.T) {
	r := require.New(t)

	var calls int
	strategy := func(ctx context.Context, t Target) (bool, error) {
		calls++
		if calls < 3 {
			return false, nil
		}
		return true, nil
	}

	err := Wait(context.Background(), newStubTarget(), strategy,
		WithInterval(20*time.Millisecond), WithTimeout(5*time.Second))
	r.NoError(err)
	r.GreaterOrEqual(calls, 3)
}

func TestWait_Timeout_NotReady(t *testing.T) {
	r := require.New(t)

	strategy := func(ctx context.Context, t Target) (bool, error) {
		return false, nil
	}

	start := time.Now()
	err := Wait(context.Background(), newStubTarget(), strategy,
		WithInterval(20*time.Millisecond), WithTimeout(150*time.Millisecond))
	r.Error(err)
	r.Contains(err.Error(), "timed out")

	// Loose bounds only: roughly ~150ms, definitely nowhere near the 60s
	// default. Avoids flakiness on slow CI.
	elapsed := time.Since(start)
	r.GreaterOrEqual(elapsed, 100*time.Millisecond)
	r.Less(elapsed, 2*time.Second)
}

func TestWait_FatalError_StopsImmediately(t *testing.T) {
	r := require.New(t)

	strategy := func(ctx context.Context, t Target) (bool, error) {
		return false, errors.New("boom")
	}

	start := time.Now()
	err := Wait(context.Background(), newStubTarget(), strategy,
		WithInterval(20*time.Millisecond), WithTimeout(30*time.Second))
	r.Error(err)
	r.Contains(err.Error(), "boom")
	// A fatal error must stop well before the 30s timeout.
	r.Less(time.Since(start), 2*time.Second)
}

func TestWait_CtxDeadlineAuthoritative(t *testing.T) {
	r := require.New(t)

	strategy := func(ctx context.Context, t Target) (bool, error) {
		return false, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := Wait(ctx, newStubTarget(), strategy,
		WithInterval(20*time.Millisecond), WithTimeout(30*time.Second))
	r.Error(err)
	r.Contains(err.Error(), "deadline exceeded")
	// The ctx deadline (100ms) wins over the much larger WithTimeout(30s).
	r.Less(time.Since(start), 5*time.Second)
}

func TestWait_CtxAlreadyDeadline_IgnoresWithTimeout(t *testing.T) {
	r := require.New(t)

	strategy := func(ctx context.Context, t Target) (bool, error) {
		return false, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := Wait(ctx, newStubTarget(), strategy,
		WithInterval(20*time.Millisecond), WithTimeout(30*time.Second))
	r.Error(err)
	r.Contains(err.Error(), "deadline exceeded")
	// An explicit caller deadline must be honored even when WithTimeout is
	// larger — so this returns long before the 30s WithTimeout.
	r.Less(time.Since(start), 5*time.Second)
}
