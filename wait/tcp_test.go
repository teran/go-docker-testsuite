package wait

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestForTCPConnection_Ready(t *testing.T) {
	r := require.New(t)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	r.NoError(err)
	defer func() { _ = l.Close() }()

	target := newStubTarget()
	target.urlResult = mustParseHostPort(t, l.Addr().String())

	strategy := ForTCPConnection(8080)
	ready, err := strategy(context.Background(), target)
	r.NoError(err)
	r.True(ready)
}

func TestForTCPConnection_NoListenerRetry(t *testing.T) {
	r := require.New(t)

	// Grab a free port, then close the listener so nothing is listening.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	r.NoError(err)
	addr := l.Addr().String()
	r.NoError(l.Close())

	target := newStubTarget()
	target.urlResult = mustParseHostPort(t, addr)

	strategy := ForTCPConnection(8080)
	ready, err := strategy(context.Background(), target)
	r.NoError(err)
	r.False(ready) // connect refused → not ready, retry (not fatal)
}

func TestForTCPConnection_PortNotRegisteredFatal(t *testing.T) {
	r := require.New(t)

	target := newStubTarget()
	target.urlErr = errors.New(`port "8080/tcp" is not registered`)

	strategy := ForTCPConnection(8080)
	_, err := strategy(context.Background(), target)
	r.Error(err) // URL error (port not registered) → fatal, no retry
}
