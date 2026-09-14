package versions

import (
	"context"
	"io"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/suite"

	"github.com/teran/go-docker-testsuite/applications/paperless-ngx"
)

// setupTimeout bounds the single Paperless-ngx boot performed in SetupSuite.
// The first start runs database migrations and model/index setup against a
// fresh PostgreSQL, which can take several minutes, so the whole suite shares
// one generous budget.
const setupTimeout = 12 * time.Minute

// clientTimeout bounds each test's own, isolated HTTP client.
const clientTimeout = 30 * time.Second

func init() {
	log.SetLevel(log.TraceLevel)
}

type PaperlessNGXTestSuite struct {
	suite.Suite

	ctx   context.Context
	image string

	app paperlessngx.PaperlessNGX
}

func New(ctx context.Context, image string) *PaperlessNGXTestSuite {
	return &PaperlessNGXTestSuite{
		ctx:   ctx,
		image: image,
	}
}

// client returns a fresh, isolated HTTP client for a single test, so test
// methods sharing the one Paperless-ngx instance do not share mutable request
// state.
func (s *PaperlessNGXTestSuite) client() *http.Client {
	return &http.Client{Timeout: clientTimeout}
}

// TestWeb verifies the Paperless-ngx web UI is served on the root path.
func (s *PaperlessNGXTestSuite) TestWeb() {
	req, err := http.NewRequestWithContext(s.ctx, http.MethodGet, s.app.MustURL(), nil)
	s.Require().NoError(err)

	resp, err := s.client().Do(req)
	s.Require().NoError(err)
	defer func() { _ = resp.Body.Close() }()

	_, _ = io.Copy(io.Discard, resp.Body)
	s.Require().Equal(http.StatusOK, resp.StatusCode)
}

// TestAPI verifies the Paperless-ngx REST API is reachable and answers HTTP 200
// when authenticated with the admin credentials (Basic auth).
func (s *PaperlessNGXTestSuite) TestAPI() {
	req, err := http.NewRequestWithContext(s.ctx, http.MethodGet, s.app.MustURL()+"/api/", nil)
	s.Require().NoError(err)
	req.SetBasicAuth(s.app.Username(), s.app.Password())

	resp, err := s.client().Do(req)
	s.Require().NoError(err)
	defer func() { _ = resp.Body.Close() }()

	_, _ = io.Copy(io.Discard, resp.Body)
	s.Require().Equal(http.StatusOK, resp.StatusCode)
}

// TestAdminCredentials verifies the wrapper exposes the deterministic admin
// credentials it created during setup (all non-empty).
func (s *PaperlessNGXTestSuite) TestAdminCredentials() {
	s.Require().NotEmpty(s.app.Username())
	s.Require().NotEmpty(s.app.Password())
}

// SetupSuite boots a single Paperless-ngx stack (paperless + postgres +
// valkey) shared by all test methods. The first boot runs DB migrations and
// model/index setup on startup, so the single generous setupTimeout is applied
// here rather than per test.
func (s *PaperlessNGXTestSuite) SetupSuite() {
	ctx, cancel := context.WithTimeout(s.ctx, setupTimeout)
	defer cancel()

	var err error
	s.app, err = paperlessngx.New(ctx, s.image)
	s.Require().NoError(err)
}

// TearDownSuite closes the shared Paperless-ngx stack once all test methods
// have run.
func (s *PaperlessNGXTestSuite) TearDownSuite() {
	if s.app == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := s.app.Close(ctx)
	s.Require().NoError(err)
}
