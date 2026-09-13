package wait

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	docker "github.com/teran/go-docker-testsuite"
)

func TestForLog_MatchingLineReady(t *testing.T) {
	r := require.New(t)

	target := &logStubTarget{
		stubTarget: newStubTarget(),
		outputFn: func(ctx context.Context, m ...docker.Matcher) ([]string, error) {
			return []string{"database system is ready to accept connections"}, nil
		},
	}

	strategy := ForLog(docker.NewSubstringMatcher("ready to accept"))
	ready, err := strategy(context.Background(), target)
	r.NoError(err)
	r.True(ready)
}

func TestForLog_NoMatchingLineRetry(t *testing.T) {
	r := require.New(t)

	target := &logStubTarget{
		stubTarget: newStubTarget(),
		outputFn: func(ctx context.Context, m ...docker.Matcher) ([]string, error) {
			return []string{"still starting up..."}, nil
		},
	}

	strategy := ForLog(docker.NewSubstringMatcher("ready to accept"))
	ready, err := strategy(context.Background(), target)
	r.NoError(err)
	r.False(ready) // snapshot has no matching line → retry
}

func TestForLog_GetOutputErrorFatal(t *testing.T) {
	r := require.New(t)

	target := &logStubTarget{
		stubTarget: newStubTarget(),
		outputFn: func(ctx context.Context, m ...docker.Matcher) ([]string, error) {
			return nil, errors.New("get output boom")
		},
	}

	strategy := ForLog(docker.NewSubstringMatcher("ready"))
	_, err := strategy(context.Background(), target)
	r.Error(err) // GetOutput infrastructure error → fatal
}

func TestForLog_NoGetOutputCapabilityFatal(t *testing.T) {
	r := require.New(t)

	// stubTarget implements only URL + Exec — it does NOT satisfy the optional
	// GetOutput capability that ForLog requires. This is a misuse → fatal.
	target := newStubTarget()

	strategy := ForLog(docker.NewSubstringMatcher("ready"))
	_, err := strategy(context.Background(), target)
	r.Error(err)
}
