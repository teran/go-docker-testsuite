package wait

import (
	"context"

	"github.com/pkg/errors"
)

// commandStrategy is the named function type returned by ForCommand. Its
// String method lets the shared poller emit a description at Debug level.
type commandStrategy func(ctx context.Context, t Target) (bool, error)

func (s commandStrategy) String() string { return "command" }

// WithExitCode sets the expected exit code. Default 0.
func WithExitCode(code int) Option {
	return func(c *config) { c.exitCode = code }
}

// ForCommand returns a strategy that is ready when running cmd inside the
// container exits with the expected code (default 0 via WithExitCode).
//
// The root package's Exec reports a non-zero exit code in the result, not as
// an error; only an infrastructure failure (create/attach/IO) is returned as
// an error and treated as fatal.
func ForCommand(cmd []string, opts ...Option) Strategy {
	cfg := defaultConfig()
	for _, o := range opts {
		o(cfg)
	}

	s := func(ctx context.Context, t Target) (bool, error) {
		res, err := t.Exec(ctx, cmd)
		if err != nil {
			return false, errors.Wrap(err, "exec command") // infra error — fatal
		}

		if res.ExitCode == cfg.exitCode {
			return true, nil
		}
		return false, nil // probe ran but not ready yet — retry
	}

	var ret commandStrategy = s
	return ret
}
