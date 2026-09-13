package wait

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestForHTTPGet_ReadyOn2xx(t *testing.T) {
	r := require.New(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	target := newStubTarget()
	target.urlResult = mustParseHostPort(t, srv.Listener.Addr().String())

	strategy := ForHTTPGet(80)
	ready, err := strategy(context.Background(), target)
	r.NoError(err)
	r.True(ready)
}

func TestForHTTPGet_RetryOn503Then200(t *testing.T) {
	r := require.New(t)

	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	target := newStubTarget()
	target.urlResult = mustParseHostPort(t, srv.Listener.Addr().String())

	strategy := ForHTTPGet(80)

	ready, err := strategy(context.Background(), target)
	r.NoError(err)
	r.False(ready) // 503 during startup → not ready, retry

	ready, err = strategy(context.Background(), target)
	r.NoError(err)
	r.True(ready) // 200 → ready
}

func TestForHTTPGet_DefaultRejects404(t *testing.T) {
	r := require.New(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	target := newStubTarget()
	target.urlResult = mustParseHostPort(t, srv.Listener.Addr().String())

	// The default accepted set is 2xx; a 404 must be a retry, not ready.
	strategy := ForHTTPGet(80)
	ready, err := strategy(context.Background(), target)
	r.NoError(err)
	r.False(ready)
}

func TestForHTTPGet_WithResponseStatusesOverridesDefault(t *testing.T) {
	r := require.New(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	target := newStubTarget()
	target.urlResult = mustParseHostPort(t, srv.Listener.Addr().String())

	// WithResponseStatuses replaces the default 2xx set: 404 becomes ready.
	strategy := ForHTTPGet(80, WithResponseStatuses(http.StatusNotFound))
	ready, err := strategy(context.Background(), target)
	r.NoError(err)
	r.True(ready)
}

func TestForHTTPGet_WithBodyContains(t *testing.T) {
	r := require.New(t)

	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			_, _ = fmt.Fprint(w, "still starting...")
			return
		}
		_, _ = fmt.Fprint(w, "cluster is all good now")
	}))
	defer srv.Close()

	target := newStubTarget()
	target.urlResult = mustParseHostPort(t, srv.Listener.Addr().String())

	strategy := ForHTTPGet(80, WithBodyContains("all good"))

	ready, err := strategy(context.Background(), target)
	r.NoError(err)
	r.False(ready) // 200 status but body lacks the substring → retry

	ready, err = strategy(context.Background(), target)
	r.NoError(err)
	r.True(ready) // body now contains substring → ready
}

func TestForHTTPGet_WithTLSReady(t *testing.T) {
	r := require.New(t)

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	target := newStubTarget()
	target.urlResult = mustParseHostPort(t, srv.Listener.Addr().String())

	strategy := ForHTTPGet(80, WithTLS(true))
	ready, err := strategy(context.Background(), target)
	r.NoError(err)
	r.True(ready)
}

func TestForHTTPGet_PlainHTTPWithTLSFatal(t *testing.T) {
	r := require.New(t)

	// Plain HTTP server; requesting https against it is a TLS handshake
	// failure → fatal configuration error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	target := newStubTarget()
	target.urlResult = mustParseHostPort(t, srv.Listener.Addr().String())

	strategy := ForHTTPGet(80, WithTLS(true))
	_, err := strategy(context.Background(), target)
	r.Error(err)
}

func TestForHTTPGet_PortNotRegisteredFatal(t *testing.T) {
	r := require.New(t)

	target := newStubTarget()
	target.urlErr = errors.New(`port "9200/tcp" is not registered`)

	strategy := ForHTTPGet(9200)
	_, err := strategy(context.Background(), target)
	r.Error(err) // URL error (port not registered) → fatal, no retry
}

func TestForHTTPGet_ConnectRefusedRetry(t *testing.T) {
	r := require.New(t)

	// Grab a free port, then close the listener so nothing is listening.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	r.NoError(err)
	addr := l.Addr().String()
	r.NoError(l.Close())

	target := newStubTarget()
	target.urlResult = mustParseHostPort(t, addr)

	strategy := ForHTTPGet(80)
	ready, err := strategy(context.Background(), target)
	r.NoError(err)
	r.False(ready) // connect refused → not ready, retry (not fatal)
}
