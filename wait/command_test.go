package wait

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	docker "github.com/teran/go-docker-testsuite"
)

func TestForCommand_ExitZeroReady(t *testing.T) {
	r := require.New(t)

	target := newStubTarget()
	target.execFn = func(ctx context.Context, cmd []string) (*docker.ExecResult, error) {
		return &docker.ExecResult{ExitCode: 0}, nil
	}

	strategy := ForCommand([]string{"true"})
	ready, err := strategy(context.Background(), target)
	r.NoError(err)
	r.True(ready)
}

func TestForCommand_NonZeroRetry(t *testing.T) {
	r := require.New(t)

	target := newStubTarget()
	target.execFn = func(ctx context.Context, cmd []string) (*docker.ExecResult, error) {
		return &docker.ExecResult{ExitCode: 1}, nil
	}

	// Default expected exit code is 0; exit 1 → not ready, retry.
	strategy := ForCommand([]string{"false"})
	ready, err := strategy(context.Background(), target)
	r.NoError(err)
	r.False(ready)
}

func TestForCommand_WithExitCode(t *testing.T) {
	r := require.New(t)

	target := newStubTarget()
	target.execFn = func(ctx context.Context, cmd []string) (*docker.ExecResult, error) {
		return &docker.ExecResult{ExitCode: 1}, nil
	}

	strategy := ForCommand([]string{"cmd"}, WithExitCode(1))
	ready, err := strategy(context.Background(), target)
	r.NoError(err)
	r.True(ready)
}

func TestForCommand_ExecErrorFatal(t *testing.T) {
	r := require.New(t)

	target := newStubTarget()
	target.execFn = func(ctx context.Context, cmd []string) (*docker.ExecResult, error) {
		return nil, errors.New("infra boom")
	}

	strategy := ForCommand([]string{"cmd"})
	_, err := strategy(context.Background(), target)
	r.Error(err) // Exec infrastructure error → fatal, no retry
}
