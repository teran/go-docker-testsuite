package paperlessngx_test

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/teran/go-docker-testsuite/applications/paperless-ngx"
	"github.com/teran/go-docker-testsuite/images"
)

func TestPaperlessNGX(t *testing.T) {
	r := require.New(t)

	ctx, cancel := context.WithTimeout(t.Context(), 12*time.Minute)
	defer cancel()

	app, err := paperlessngx.New(ctx, images.PaperlessNGX)
	r.NoError(err)
	defer func() { _ = app.Close(ctx) }()

	r.NotEmpty(app.Username())
	r.NotEmpty(app.Password())

	// Web root must answer 200.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, app.MustURL(), nil)
	r.NoError(err)

	cli := &http.Client{Timeout: 30 * time.Second}
	resp, err := cli.Do(req)
	r.NoError(err)
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	r.Equal(http.StatusOK, resp.StatusCode)

	// API root with Basic auth must answer 200.
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, app.MustURL()+"/api/", nil)
	r.NoError(err)
	req.SetBasicAuth(app.Username(), app.Password())

	resp, err = cli.Do(req)
	r.NoError(err)
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	r.Equal(http.StatusOK, resp.StatusCode)
}
