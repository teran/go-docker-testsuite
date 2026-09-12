package wait

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestForAll_ReadyWhenAllReady(t *testing.T) {
	r := require.New(t)

	target := newStubTarget()
	ready := func(ctx context.Context, t Target) (bool, error) { return true, nil }

	strategy := ForAll(ready, ready, ready)
	got, err := strategy(context.Background(), target)
	r.NoError(err)
	r.True(got)
}

func TestForAll_NotReadyWhenAnyNotReady(t *testing.T) {
	r := require.New(t)

	target := newStubTarget()
	ready := func(ctx context.Context, t Target) (bool, error) { return true, nil }
	notReady := func(ctx context.Context, t Target) (bool, error) { return false, nil }

	strategy := ForAll(ready, notReady, ready)
	got, err := strategy(context.Background(), target)
	r.NoError(err)
	r.False(got)
}

func TestForAny_ReadyWhenAny(t *testing.T) {
	r := require.New(t)

	target := newStubTarget()
	ready := func(ctx context.Context, t Target) (bool, error) { return true, nil }
	notReady := func(ctx context.Context, t Target) (bool, error) { return false, nil }

	strategy := ForAny(notReady, ready, notReady)
	got, err := strategy(context.Background(), target)
	r.NoError(err)
	r.True(got)
}

func TestForAtLeast_ReadyAtThreshold(t *testing.T) {
	r := require.New(t)

	target := newStubTarget()
	ready := func(ctx context.Context, t Target) (bool, error) { return true, nil }
	notReady := func(ctx context.Context, t Target) (bool, error) { return false, nil }

	// 2 of 3 ready → satisfies ForAtLeast(2).
	strategy := ForAtLeast(2, ready, notReady, ready)
	got, err := strategy(context.Background(), target)
	r.NoError(err)
	r.True(got)
}

func TestForAtLeast_ZeroImmediate(t *testing.T) {
	r := require.New(t)

	target := newStubTarget()
	notReady := func(ctx context.Context, t Target) (bool, error) { return false, nil }

	// x <= 0 is vacuous success: immediately ready.
	strategy := ForAtLeast(0, notReady, notReady)
	got, err := strategy(context.Background(), target)
	r.NoError(err)
	r.True(got)
}

func TestForAtLeast_XGreaterThanLen_TimesOut(t *testing.T) {
	r := require.New(t)

	target := newStubTarget()
	a := func(ctx context.Context, t Target) (bool, error) { return true, nil }
	b := func(ctx context.Context, t Target) (bool, error) { return true, nil }

	// 3 required but only 2 strategies → can never be satisfied → times out.
	strategy := ForAtLeast(3, a, b)
	err := Wait(context.Background(), target, strategy,
		WithInterval(20*time.Millisecond), WithTimeout(120*time.Millisecond))
	r.Error(err)
	r.Contains(err.Error(), "timed out")
}

func TestForAll_EmptyImmediate(t *testing.T) {
	r := require.New(t)

	target := newStubTarget()

	// ForAll() with no strategies is vacuously ready immediately.
	strategy := ForAll()
	got, err := strategy(context.Background(), target)
	r.NoError(err)
	r.True(got)
}

func TestForAny_EmptyTimesOut(t *testing.T) {
	r := require.New(t)

	target := newStubTarget()

	// ForAny() with no strategies is never satisfiable → times out.
	strategy := ForAny()
	err := Wait(context.Background(), target, strategy,
		WithInterval(20*time.Millisecond), WithTimeout(120*time.Millisecond))
	r.Error(err)
	r.Contains(err.Error(), "timed out")
}

func TestForAll_FatalErrorFailsPoll(t *testing.T) {
	r := require.New(t)

	target := newStubTarget()
	fatal := func(ctx context.Context, t Target) (bool, error) {
		return false, errors.New("boom")
	}
	slow := func(ctx context.Context, t Target) (bool, error) {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(5 * time.Second):
			return true, nil
		}
	}

	// A fatal error in one sub-strategy cancels the others and fails the poll
	// promptly (not via the 30s global timeout).
	start := time.Now()
	strategy := ForAll(fatal, slow)
	err := Wait(context.Background(), target, strategy,
		WithInterval(20*time.Millisecond), WithTimeout(30*time.Second))
	r.Error(err)
	r.Contains(err.Error(), "boom")
	r.Less(time.Since(start), 2*time.Second)
}

func TestForAll_SubStrategiesConcurrent(t *testing.T) {
	r := require.New(t)

	target := newStubTarget()

	var mu sync.Mutex
	var active int
	var concurrent bool

	slow := func(ctx context.Context, t Target) (bool, error) {
		mu.Lock()
		active++
		if active > 1 {
			concurrent = true
		}
		mu.Unlock()

		time.Sleep(50 * time.Millisecond)

		mu.Lock()
		active--
		mu.Unlock()

		return true, nil
	}

	// Three 50ms strategies must overlap; if they ran sequentially, active
	// would never exceed 1.
	strategy := ForAll(slow, slow, slow)
	got, err := strategy(context.Background(), target)
	r.NoError(err)
	r.True(got)
	r.True(concurrent)
}
