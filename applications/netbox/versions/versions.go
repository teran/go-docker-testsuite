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

// setupTimeout bounds a single NetBox boot. The first start runs ~200 database
// migrations against a fresh PostgreSQL, which can take several minutes, so each
// test gets its own generous budget rather than sharing one suite-wide deadline.
const setupTimeout = 12 * time.Minute

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

// TestWeb verifies the NetBox web UI is served on the root path.
func (s *NetBoxTestSuite) TestWeb() {
	resp, err := http.Get(s.app.MustURL())
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

	resp, err := http.DefaultClient.Do(req)
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
	resp, err := http.Get(s.app.MustURL() + "/api/")
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

func (s *NetBoxTestSuite) SetupTest() {
	// Each test boots a fresh NetBox stack that runs DB migrations on startup.
	// Derive an independent, generous timeout per test so one test's migration
	// time doesn't consume a shared suite deadline.
	ctx, cancel := context.WithTimeout(s.ctx, setupTimeout)
	defer cancel()

	var err error
	s.app, err = netbox.New(ctx, s.image)
	s.Require().NoError(err)
}

func (s *NetBoxTestSuite) TearDownTest() {
	if s.app == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := s.app.Close(ctx)
	s.Require().NoError(err)
}
