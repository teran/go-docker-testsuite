package versions

import (
	"context"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/suite"

	"github.com/teran/go-docker-testsuite/v2/applications/scylladb"
)

const (
	ScyllaDBTestDefaultTimeout = 3 * time.Minute
	CleanupDefaultTimeout      = 1 * time.Minute
)

type testSuite struct {
	suite.Suite

	sdb   scylladb.ScyllaDB
	image string
}

func init() {
	log.SetLevel(log.TraceLevel)
}

func (s *testSuite) TestAll() {
	err := s.sdb.CreateKeyspace("somedb")
	s.Require().NoError(err)

	err = s.sdb.DropKeyspace("notexistentdb")
	s.Require().Error(err)

	sc, err := s.sdb.ClusterConfig("somedb")
	s.Require().NoError(err)

	session, err := sc.CreateSession()
	s.Require().NoError(err)

	err = session.Query(`CREATE TABLE testtable (id int, item text, primary key(id));`).Exec()
	s.Require().NoError(err)

	err = session.Query(`INSERT INTO testtable(id, item) VALUES (?, ?);`, 1, "test").Exec()
	s.Require().NoError(err)

	err = s.sdb.DropKeyspace("somedb")
	s.Require().NoError(err)
}

// ========================================================================
// Test suite setup
// ========================================================================
func New(ctx context.Context, image string) *testSuite {
	return &testSuite{
		image: image,
	}
}

func (s *testSuite) SetupSuite() {
	var err error
	s.sdb, err = scylladb.NewWithImage(s.T().Context(), s.image)
	s.Require().NoError(err)
}

func (s *testSuite) TearDownSuite() {
	ctx, cancel := context.WithTimeout(s.T().Context(), CleanupDefaultTimeout)
	defer cancel()

	err := s.sdb.Close(ctx)
	s.Require().NoError(err)
}
