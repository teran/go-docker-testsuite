package wait

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/errors"

	docker "github.com/teran/go-docker-testsuite"
)

// httpGetStrategy is the named function type returned by ForHTTPGet. Its
// String method lets the shared poller emit a description at Debug level.
type httpGetStrategy func(ctx context.Context, t Target) (bool, error)

func (s httpGetStrategy) String() string { return "http-get" }

// WithPath sets the request path. Default "/".
func WithPath(p string) Option {
	return func(c *config) { c.path = p }
}

// WithMethod sets the HTTP method. Default GET.
func WithMethod(m string) Option {
	return func(c *config) { c.method = m }
}

// WithResponseStatuses overrides the accepted status set (default: any 2xx).
func WithResponseStatuses(codes ...int) Option {
	return func(c *config) { c.statuses = codes }
}

// WithTLS selects the https scheme. When true the per-request client trusts a
// self-signed certificate (InsecureSkipVerify) so the strategy can probe test
// TLS servers. Default false (http).
func WithTLS(b bool) Option {
	return func(c *config) { c.tls = b }
}

// WithBodyContains requires the response body to contain substr for the probe
// to be ready. Default "" (no body check).
func WithBodyContains(substr string) Option {
	return func(c *config) { c.bodySubstr = substr }
}

// ForHTTPGet returns a strategy that is ready when an HTTP request to the
// container's internal port returns an accepted status (default 2xx,
// overridable via WithResponseStatuses) and, if WithBodyContains is set, a
// body containing the substring.
//
// "connection refused" (server not listening yet) and unaccepted statuses are
// transient retries. An invalid URL or a TLS handshake failure is fatal, as is
// t.URL reporting the port as not registered.
func ForHTTPGet(port uint16, opts ...Option) Strategy {
	cfg := defaultConfig()
	for _, o := range opts {
		o(cfg)
	}

	client := &http.Client{Timeout: 1 * time.Second}
	if cfg.tls {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // trust test TLS servers
		}
	}

	s := func(ctx context.Context, t Target) (bool, error) {
		hp, err := t.URL(docker.ProtoTCP, port)
		if err != nil {
			return false, errors.Wrap(err, "resolve URL")
		}

		scheme := "http"
		if cfg.tls {
			scheme = "https"
		}
		url := fmt.Sprintf("%s://%s%s", scheme, hp.String(), cfg.path)

		req, err := http.NewRequestWithContext(ctx, cfg.method, url, nil)
		if err != nil {
			return false, errors.Wrap(err, "build request")
		}

		resp, err := client.Do(req)
		if err != nil {
			if isConnRefused(err) {
				return false, nil // server not listening yet — retry
			}
			return false, errors.Wrap(err, "http request") // e.g. TLS handshake failure — fatal
		}
		defer func() { _ = resp.Body.Close() }()

		if !acceptedStatus(cfg.statuses, resp.StatusCode) {
			return false, nil // server up but not ready — retry
		}

		if cfg.bodySubstr != "" {
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return false, errors.Wrap(err, "read body")
			}
			if !strings.Contains(string(body), cfg.bodySubstr) {
				return false, nil // accepted status but body mismatch — retry
			}
		}

		return true, nil
	}

	var ret httpGetStrategy = s
	return ret
}

// acceptedStatus reports whether code is accepted. A nil statuses set means any
// 2xx; otherwise code must be explicitly listed.
func acceptedStatus(statuses []int, code int) bool {
	if statuses == nil {
		return code >= 200 && code <= 299
	}
	for _, c := range statuses {
		if c == code {
			return true
		}
	}
	return false
}

// isConnRefused reports whether err is a "connection refused" dial failure.
// It is deliberately narrow (ECONNREFUSED only) so that other transport errors
// such as TLS handshake failures are classified as fatal rather than retried.
func isConnRefused(err error) bool {
	return errors.Is(err, syscall.ECONNREFUSED)
}
