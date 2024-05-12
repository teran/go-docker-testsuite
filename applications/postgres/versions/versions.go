package versions

import (
	"context"

	"github.com/stretchr/testify/suite"

	"github.com/teran/go-docker-testsuite/applications/postgres"
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
	pg, err := postgres.NewWithImage(s.ctx, s.image)
	s.Require().NoError(err)

	defer func() {
		err := pg.Close(s.ctx)
		s.Require().NoError(err)
	}()

	err = pg.CreateDB(s.ctx, "somedb")
	s.Require().NoError(err)

	err = pg.DropDB(s.ctx, "anotherdb")
	s.Require().Error(err)

	err = pg.DropDB(s.ctx, "somedb")
	s.Require().NoError(err)
}
