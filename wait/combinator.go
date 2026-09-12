package wait

import (
	"context"
	"sync"

	"golang.org/x/sync/errgroup"
)

// atLeastStrategy is the named function type returned by the combinators. Its
// String method lets the shared poller emit a description at Debug level.
type atLeastStrategy func(ctx context.Context, t Target) (bool, error)

func (s atLeastStrategy) String() string { return "at-least" }

// ForAtLeast returns a strategy that is ready when at least x of the
// sub-strategies are ready. Sub-strategies run concurrently for each poll via
// errgroup. Edge cases: x <= 0 is vacuous success; x > len(strategies) can
// never be satisfied (the poller times out).
func ForAtLeast(x int, strategies ...Strategy) Strategy {
	s := func(ctx context.Context, t Target) (bool, error) {
		if x <= 0 {
			return true, nil // vacuous success
		}
		if x > len(strategies) {
			return false, nil // can never be satisfied — retry until timeout
		}

		g, gctx := errgroup.WithContext(ctx)

		var mu sync.Mutex
		readyCount := 0

		for _, strat := range strategies {
			strat := strat
			g.Go(func() error {
				ready, err := strat(gctx, t)
				if err != nil {
					return err // fatal — cancels the group
				}
				if ready {
					mu.Lock()
					readyCount++
					mu.Unlock()
				}
				return nil
			})
		}

		if err := g.Wait(); err != nil {
			return false, err // fatal
		}

		return readyCount >= x, nil
	}

	var ret atLeastStrategy = s
	return ret
}

// ForAll returns a strategy that is ready when all sub-strategies are ready. An
// empty list is vacuously ready immediately.
func ForAll(strategies ...Strategy) Strategy {
	return ForAtLeast(len(strategies), strategies...)
}

// ForAny returns a strategy that is ready when at least one sub-strategy is
// ready. An empty list is never satisfiable and always times out.
func ForAny(strategies ...Strategy) Strategy {
	return ForAtLeast(1, strategies...)
}
