package wait

import (
	"context"

	"github.com/pkg/errors"

	docker "github.com/teran/go-docker-testsuite"
)

// logOutputer is the optional capability ForLog needs from a Target. Both
// docker.Container and docker.TestContainer satisfy it.
type logOutputer interface {
	GetOutput(ctx context.Context, m ...docker.Matcher) ([]string, error)
}

// logStrategy is the named function type returned by ForLog. Its String method
// lets the shared poller emit a description at Debug level on retry.
type logStrategy func(ctx context.Context, t Target) (bool, error)

func (s logStrategy) String() string { return "log-match" }

// ForLog returns a strategy that is ready when some line of container output
// matches m. The Target must implement the optional GetOutput capability;
// otherwise ForLog returns a fatal error.
func ForLog(m docker.Matcher, opts ...Option) Strategy {
	_ = opts // no strategy-specific options currently

	s := func(ctx context.Context, t Target) (bool, error) {
		o, ok := t.(logOutputer)
		if !ok {
			return false, errors.New("target does not implement log output capability")
		}

		lines, err := o.GetOutput(ctx, m)
		if err != nil {
			return false, errors.Wrap(err, "get output")
		}

		for _, l := range lines {
			if m(l) {
				return true, nil
			}
		}
		return false, nil
	}

	var ret logStrategy = s
	return ret
}
