package wait

import (
	"context"
	"net"
	"time"

	"github.com/pkg/errors"

	docker "github.com/teran/go-docker-testsuite"
)

// tcpStrategy is the named function type returned by ForTCPConnection. Its
// String method lets the shared poller emit a description at Debug level.
type tcpStrategy func(ctx context.Context, t Target) (bool, error)

func (s tcpStrategy) String() string { return "tcp-connection" }

// ForTCPConnection returns a strategy that is ready when a TCP connection to
// the container's internal port can be established (and is immediately closed).
// This is a pure liveness probe: it proves the listener accepts connections but
// says nothing about application readiness.
//
// A refused/unreachable connect is a transient retry; t.URL reporting the port
// as not registered is fatal. UDP is not supported: there is no acknowledgement
// on a UDP "connection", so a connect-style liveness probe is meaningless.
func ForTCPConnection(port uint16, opts ...Option) Strategy {
	_ = opts // no strategy-specific options currently

	s := func(ctx context.Context, t Target) (bool, error) {
		hp, err := t.URL(docker.ProtoTCP, port)
		if err != nil {
			return false, errors.Wrap(err, "resolve URL") // port not registered — fatal
		}

		dialer := &net.Dialer{Timeout: 1 * time.Second}
		conn, err := dialer.DialContext(ctx, "tcp", hp.String())
		if err != nil {
			return false, nil // not listening yet — retry
		}
		_ = conn.Close()
		return true, nil
	}

	var ret tcpStrategy = s
	return ret
}
