package versions

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/suite"

	"github.com/teran/go-docker-testsuite/applications/forgejo"
)

func init() {
	log.SetLevel(log.TraceLevel)
}

type ForgejoTestSuite struct {
	suite.Suite

	ctx   context.Context
	image string

	app forgejo.Forgejo
}

func New(ctx context.Context, image string) *ForgejoTestSuite {
	return &ForgejoTestSuite{
		ctx:   ctx,
		image: image,
	}
}

// TestWeb verifies the web UI is served on the root path.
func (s *ForgejoTestSuite) TestWeb() {
	resp, err := http.Get(s.app.MustURL())
	s.Require().NoError(err)
	defer func() { _ = resp.Body.Close() }()

	_, _ = io.Copy(io.Discard, resp.Body)
	s.Require().Equal(http.StatusOK, resp.StatusCode)
}

// TestAPI verifies the Forgejo REST API is reachable (GET /api/v1/version).
func (s *ForgejoTestSuite) TestAPI() {
	resp, err := http.Get(s.app.MustURL() + "/api/v1/version")
	s.Require().NoError(err)
	defer func() { _ = resp.Body.Close() }()

	s.Require().Equal(http.StatusOK, resp.StatusCode)

	var v struct {
		Version string `json:"version"`
	}
	s.Require().NoError(json.NewDecoder(resp.Body).Decode(&v))
	s.Require().NotEmpty(v.Version)
}

// TestSSH verifies the Forgejo SSH endpoint accepts connections and speaks the
// SSH protocol (the server sends its version banner on connect).
func (s *ForgejoTestSuite) TestSSH() {
	conn, err := net.DialTimeout("tcp", s.app.MustSSHAddr(), 10*time.Second)
	s.Require().NoError(err)
	defer func() { _ = conn.Close() }()

	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

	banner, err := bufio.NewReader(conn).ReadString('\n')
	s.Require().NoError(err)
	s.Require().True(strings.HasPrefix(banner, "SSH-2.0-"))
}

func (s *ForgejoTestSuite) SetupTest() {
	var err error
	s.app, err = forgejo.New(s.ctx, s.image)
	s.Require().NoError(err)
}

func (s *ForgejoTestSuite) TearDownTest() {
	err := s.app.Close(s.ctx)
	s.Require().NoError(err)
}
