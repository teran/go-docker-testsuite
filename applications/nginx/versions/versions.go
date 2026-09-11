// Package versions provides a versioned integration test suite for the nginx
// wrapper. Each concrete version pins an image and runs the suite, so every
// supported nginx image is exercised against the same scenario.
package versions

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/teran/go-docker-testsuite/applications/nginx"
)

type testSuite struct {
	suite.Suite

	ctx   context.Context
	image string
}

// New returns a versioned nginx suite bound to the given context and image.
func New(ctx context.Context, image string) *testSuite {
	return &testSuite{
		ctx:   ctx,
		image: image,
	}
}

// TestConfigInjection is a self-contained scenario that does not need a host
// goroutine: nginx is started with a custom config via WithImage(image) and we
// assert the injected `return 200 "versioned"` body comes back. A random
// listen port is used to avoid conflicts on the host.
func (s *testSuite) TestConfigInjection() {
	r := s.Require()

	// nginx config injection + a random listen port so multiple versioned
	// suites can run in parallel without clashing on port 80.
	config := []byte(`server {
    listen 80;
    location / {
        return 200 "versioned";
    }
}
`)

	app, err := nginx.NewWithConfig(s.ctx, config, nginx.WithImage(s.image))
	r.NoError(err)
	r.NotNil(app)

	defer func() {
		err := app.Close(s.ctx)
		r.NoError(err)
	}()

	addr := app.MustAddr()
	r.NotEmpty(addr)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://%s/", addr))
	r.NoError(err)
	defer func() { _ = resp.Body.Close() }()

	r.Equal(http.StatusOK, resp.StatusCode)

	data, err := io.ReadAll(resp.Body)
	r.NoError(err)
	r.Equal("versioned", string(data))
}
