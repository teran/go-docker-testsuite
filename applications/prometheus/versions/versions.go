package versions

import (
	"context"

	"github.com/stretchr/testify/suite"

	"github.com/teran/go-docker-testsuite/applications/prometheus"
)

type testSuite struct {
	suite.Suite

	ctx   context.Context
	image string
}

func New(ctx context.Context, image string) *testSuite {
	return &testSuite{
		ctx:   ctx,
		image: image,
	}
}

func (s *testSuite) TestAll() {
	app, err := prometheus.NewWithImage(s.ctx, s.image)
	s.Require().NoError(err)

	defer func() {
		err := app.Close(s.ctx)
		s.Require().NoError(err)
	}()

	bi, err := app.Client().Buildinfo(s.ctx)
	s.Require().NoError(err)
	s.Require().NotEmpty(bi.Version)
}
