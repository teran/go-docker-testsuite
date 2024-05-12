package versions

import (
	"context"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/suite"

	"github.com/teran/go-docker-testsuite/applications/scylladb"
)

const (
	ScyllaDBTestDefaultTimeout = 3 * time.Minute
	CleanupDefaultTimeout      = 1 * time.Minute
)

type testSuite struct {
	suite.Suite

	ctx   context.Context
	sdb   scylladb.ScyllaDB
	image string
}

func init() {
	log.SetLevel(log.TraceLevel)
}

func (s *testSuite) TestAll() {
	sc, err := s.sdb.ClusterConfig("")
	s.Require().NoError(err)
	s.Require().NotNil(sc)

	err = s.sdb.CreateKeyspace("somedb")
	s.Require().NoError(err)

	err = s.sdb.DropKeyspace("notexistentdb")
	s.Require().Error(err)

	err = s.sdb.DropKeyspace("somedb")
	s.Require().NoError(err)
}

// ========================================================================
// Test suite setup
// ========================================================================
func New(ctx context.Context, image string) *testSuite {
	return &testSuite{
		ctx:   ctx,
		image: image,
	}
}

func (s *testSuite) SetupSuite() {
	var err error
	s.sdb, err = scylladb.NewWithImage(s.ctx, s.image)
	s.Require().NoError(err)
}

func (s *testSuite) TearDownSuite() {
	ctx, cancel := context.WithTimeout(context.TODO(), CleanupDefaultTimeout)
	defer cancel()

	err := s.sdb.Close(ctx)
	s.Require().NoError(err)
}
