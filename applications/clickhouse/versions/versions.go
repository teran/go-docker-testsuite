package versions

import (
	"context"
	"database/sql"

	_ "github.com/ClickHouse/clickhouse-go/v2" // registers the "clickhouse" driver

	"github.com/stretchr/testify/suite"

	"github.com/teran/go-docker-testsuite/applications/clickhouse"
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
	ch, err := clickhouse.NewWithImage(s.ctx, s.image)
	s.Require().NoError(err)

	defer func() {
		err := ch.Close(s.ctx)
		s.Require().NoError(err)
	}()

	err = ch.CreateDatabase(s.ctx, "somedb")
	s.Require().NoError(err)

	db, err := sql.Open("clickhouse", ch.MustDSN("somedb"))
	s.Require().NoError(err)
	defer func() { _ = db.Close() }()

	var result int
	s.Require().NoError(db.QueryRowContext(s.ctx, "SELECT 1").Scan(&result))
	s.Require().Equal(1, result)

	err = ch.DropDatabase(s.ctx, "somedb")
	s.Require().NoError(err)
}
