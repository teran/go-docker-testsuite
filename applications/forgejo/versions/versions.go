package versions

import (
	"context"
	"io"
	"net/http"

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

func (s *ForgejoTestSuite) TestForgejo() {
	resp, err := http.Get(s.app.MustURL())
	s.Require().NoError(err)
	defer func() { _ = resp.Body.Close() }()

	_, _ = io.Copy(io.Discard, resp.Body)
	s.Require().Equal(http.StatusOK, resp.StatusCode)
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
