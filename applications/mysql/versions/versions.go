package versions

import (
	"context"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/suite"
	"github.com/teran/go-docker-testsuite/v2/applications/mysql"
)

func init() {
	log.SetLevel(log.TraceLevel)
}

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
	ms, err := mysql.New(s.ctx, s.image)
	s.Require().NoError(err)

	defer func() {
		err := ms.Close(s.ctx)
		s.Require().NoError(err)
	}()

	err = ms.CreateDB(s.ctx, "somedb")
	s.Require().NoError(err)

	err = ms.DropDB(s.ctx, "anotherdb")
	s.Require().Error(err)

	err = ms.DropDB(s.ctx, "somedb")
	s.Require().NoError(err)
}
