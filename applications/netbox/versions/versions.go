package versions

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/suite"

	"github.com/teran/go-docker-testsuite/applications/netbox"
)

// setupTimeout bounds the single NetBox boot performed in SetupSuite. The
// first start runs ~200 database migrations against a fresh PostgreSQL, which
// can take several minutes, so the whole suite shares one generous budget.
const setupTimeout = 12 * time.Minute

// clientTimeout bounds each test's own, isolated HTTP client.
const clientTimeout = 30 * time.Second

func init() {
	log.SetLevel(log.TraceLevel)
}

type NetBoxTestSuite struct {
	suite.Suite

	ctx   context.Context
	image string

	app netbox.NetBox
}

func New(ctx context.Context, image string) *NetBoxTestSuite {
	return &NetBoxTestSuite{
		ctx:   ctx,
		image: image,
	}
}

// client returns a fresh, isolated HTTP client for a single test, so test
// methods sharing the one NetBox instance do not share mutable request state.
func (s *NetBoxTestSuite) client() *http.Client {
	return &http.Client{Timeout: clientTimeout}
}

// TestWeb verifies the NetBox web UI is served on the root path.
func (s *NetBoxTestSuite) TestWeb() {
	req, err := http.NewRequestWithContext(s.ctx, http.MethodGet, s.app.MustURL(), nil)
	s.Require().NoError(err)

	resp, err := s.client().Do(req)
	s.Require().NoError(err)
	defer func() { _ = resp.Body.Close() }()

	_, _ = io.Copy(io.Discard, resp.Body)
	s.Require().Equal(http.StatusOK, resp.StatusCode)
}

// TestAPI verifies the NetBox REST API is reachable and answers JSON when
// authenticated with the superuser API token. NetBox 4.6 authenticates v2 API
// tokens with the "Bearer" scheme.
func (s *NetBoxTestSuite) TestAPI() {
	req, err := http.NewRequestWithContext(s.ctx, http.MethodGet, s.app.MustURL()+"/api/", nil)
	s.Require().NoError(err)
	req.Header.Set("Authorization", "Bearer "+s.app.SuperuserAPIToken())

	resp, err := s.client().Do(req)
	s.Require().NoError(err)
	defer func() { _ = resp.Body.Close() }()

	s.Require().Equal(http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	s.Require().NoError(json.NewDecoder(resp.Body).Decode(&body))
	s.Require().NotEmpty(body)
}

// TestAPIUnauthorized verifies the NetBox REST API rejects anonymous callers.
// NetBox responds 403 to unauthenticated requests to the API root (see the
// wrapper's probeAPI, which relies on the same behaviour).
func (s *NetBoxTestSuite) TestAPIUnauthorized() {
	req, err := http.NewRequestWithContext(s.ctx, http.MethodGet, s.app.MustURL()+"/api/", nil)
	s.Require().NoError(err)

	resp, err := s.client().Do(req)
	s.Require().NoError(err)
	defer func() { _ = resp.Body.Close() }()

	_, _ = io.Copy(io.Discard, resp.Body)
	s.Require().Equal(http.StatusForbidden, resp.StatusCode)
}

// TestSuperuserCredentials verifies the wrapper exposes the deterministic
// superuser credentials it created during setup (all non-empty).
func (s *NetBoxTestSuite) TestSuperuserCredentials() {
	s.Require().NotEmpty(s.app.SuperuserUsername())
	s.Require().NotEmpty(s.app.SuperuserPassword())
	s.Require().NotEmpty(s.app.SuperuserAPIToken())
}

// SetupSuite boots a single NetBox stack (netbox + postgres + redis) shared by
// all test methods. The first boot runs DB migrations on startup, so the single
// generous setupTimeout is applied here rather than per test.
func (s *NetBoxTestSuite) SetupSuite() {
	ctx, cancel := context.WithTimeout(s.ctx, setupTimeout)
	defer cancel()

	var err error
	s.app, err = netbox.New(ctx, s.image)
	s.Require().NoError(err)
}

// TearDownSuite closes the shared NetBox stack once all test methods have run.
func (s *NetBoxTestSuite) TearDownSuite() {
	if s.app == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := s.app.Close(ctx)
	s.Require().NoError(err)
}
