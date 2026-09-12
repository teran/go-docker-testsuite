package wait

import (
	"context"
	"math"
	"net"
	"strconv"
	"sync"
	"testing"

	docker "github.com/teran/go-docker-testsuite"
)

// stubTarget is an in-memory wait.Target implementing only URL and Exec. It
// deliberately does NOT implement the optional GetOutput capability, so it can
// be used to exercise the ForLog capability-misuse (fatal) path.
//
// This is a pure/unit test double for the wait package's poller and
// combinators; it is explicitly allowed by DESIGN.md §7.1 (the "no mocks" rule
// targets container/integration tests, not these deterministic unit tests).
type stubTarget struct {
	urlResult *docker.HostPort
	urlErr    error

	mu        sync.Mutex
	execFn    func(ctx context.Context, cmd []string) (*docker.ExecResult, error)
	execCalls int
}

func newStubTarget() *stubTarget {
	return &stubTarget{}
}

func (s *stubTarget) URL(proto docker.Protocol, port uint16) (*docker.HostPort, error) {
	return s.urlResult, s.urlErr
}

func (s *stubTarget) Exec(ctx context.Context, cmd []string) (*docker.ExecResult, error) {
	s.mu.Lock()
	s.execCalls++
	s.mu.Unlock()

	if s.execFn == nil {
		return &docker.ExecResult{ExitCode: 0}, nil
	}
	return s.execFn(ctx, cmd)
}

// logStubTarget adds the optional GetOutput capability used by ForLog on top
// of a stubTarget. It satisfies the internal logOutputer interface.
type logStubTarget struct {
	*stubTarget
	outputFn func(ctx context.Context, m ...docker.Matcher) ([]string, error)
}

func (s *logStubTarget) GetOutput(ctx context.Context, m ...docker.Matcher) ([]string, error) {
	return s.outputFn(ctx, m...)
}

// mustParseHostPort parses a "host:port" address (e.g. an httptest server's
// listener address) into a docker.HostPort.
func mustParseHostPort(t *testing.T, addr string) *docker.HostPort {
	t.Helper()

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split host:port %q: %v", addr, err)
	}

	p, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port %q: %v", portStr, err)
	}
	if p < 0 || p > math.MaxUint16 {
		panic("port out of range")
	}

	return &docker.HostPort{Host: host, Port: uint16(p)}
}
