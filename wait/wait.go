// Package wait provides composable readiness wait-strategies for Docker
// containers. It generalizes the library's log-line readiness model into
// reusable predicates (HTTP probes, in-container commands, raw TCP connects,
// log matching) combined with ForAll / ForAny / ForAtLeast.
//
// A Strategy is a pure function predicate with strict (bool, error) semantics:
//
//	(true, nil)  = ready now
//	(false, nil) = not ready yet (retry)
//	(_, err)     = fatal — stop immediately
//
// Wait owns the single shared poll loop; strategies never loop on their own.
// Timeouts and intervals are global to a Wait call, not per-strategy.
package wait

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	docker "github.com/teran/go-docker-testsuite"
)

// Target is the minimal capability a strategy needs from a container. Both
// docker.Container and docker.TestContainer satisfy it directly.
type Target interface {
	URL(proto docker.Protocol, port uint16) (*docker.HostPort, error)
	Exec(ctx context.Context, cmd []string) (*docker.ExecResult, error)
}

// Strategy is a readiness predicate. See the (bool, error) semantics table in
// the package documentation. It is a type alias (not a named type) so that
// one-line inline strategies are assignable while named strategies may still
// carry a String() method for diagnostics.
type Strategy = func(ctx context.Context, t Target) (bool, error)

// config carries both the global (Wait-owned) and strategy-specific fields.
// Each consumer applies the same ...Option to a fresh config and reads only
// the fields it owns.
type config struct {
	// global — read ONLY by Wait
	timeout  time.Duration // default 60s
	interval time.Duration // default 200ms

	// ForHTTPGet — read ONLY by ForHTTPGet
	path       string // default "/"
	method     string // default GET
	statuses   []int  // default: any 2xx (nil)
	tls        bool   // default false (http scheme)
	bodySubstr string // default "" (no body check)

	// ForCommand — read ONLY by ForCommand
	exitCode int // default 0
}

// defaultConfig returns a fresh config populated with the agreed defaults.
func defaultConfig() *config {
	return &config{
		timeout:  60 * time.Second,
		interval: 200 * time.Millisecond,
		path:     "/",
		method:   "GET",
		statuses: nil,
		exitCode: 0,
	}
}

// Option configures a Wait call or a strategy constructor. Options that do not
// apply to a particular consumer are ignored (see DESIGN §2.4).
type Option func(*config)

// WithInterval sets the poll interval. Non-positive values are clamped to the
// default (200ms). Default: 200ms.
func WithInterval(d time.Duration) Option {
	return func(c *config) {
		if d > 0 {
			c.interval = d
		}
	}
}

// WithTimeout bounds the whole Wait call when ctx carries no deadline. An
// explicit ctx deadline is authoritative and overrides this. Non-positive
// values are clamped to the default (60s). Default: 60s.
func WithTimeout(d time.Duration) Option {
	return func(c *config) {
		if d > 0 {
			c.timeout = d
		}
	}
}

// Wait runs a single strategy against t via the shared poller, honouring the
// ctx deadline. It returns nil as soon as the strategy reports ready, or a
// wrapped error on a fatal strategy error or on the deadline/timeout elapsing.
func Wait(ctx context.Context, t Target, s Strategy, opts ...Option) error {
	cfg := defaultConfig()
	for _, o := range opts {
		o(cfg)
	}

	// Deadline resolution: an explicit ctx deadline is authoritative; otherwise
	// bound the wait with WithTimeout.
	waitCtx := ctx
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		waitCtx, cancel = context.WithTimeout(ctx, cfg.timeout)
		defer cancel()
	}

	for {
		ready, err := s(waitCtx, t)
		if err != nil {
			return errors.Wrap(err, "wait strategy failed") // fatal
		}
		if ready {
			return nil
		}

		// Not ready yet — optionally log the strategy description, then wait.
		// Strategy is a func type alias (not an interface), so convert to
		// interface{} before asserting; named strategies carry String().
		if d, ok := interface{}(s).(fmt.Stringer); ok {
			log.WithField("strategy", d.String()).Debug("wait strategy not ready; retrying")
		} else {
			log.Trace("wait strategy not ready; retrying")
		}

		select {
		case <-waitCtx.Done():
			return errors.Wrapf(waitCtx.Err(), "waiting for strategy to become ready timed out")
		case <-time.After(cfg.interval):
		}
	}
}
