package postgres

import (
	"context"
	"strings"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"

	pgx "github.com/jackc/pgx/v4"
	"github.com/stretchr/testify/suite"
)

const (
	dbName         = "testdb"
	testTimeout    = 3 * time.Minute
	cleanupTimeout = 1 * time.Minute
)

func init() {
	log.SetLevel(log.TraceLevel)
}

func (s *postgresTestSuite) TestAll() {
	dsn, err := s.pg.DSN("somedb")
	s.Require().NoError(err)
	s.Require().NotEmpty(dsn)
	s.True(strings.HasPrefix(dsn, "postgres://"))
	s.True(strings.HasSuffix(dsn, "/somedb?sslmode=disable"))

	err = s.pg.CreateDB(s.ctx, dbName)
	s.Require().NoError(err)

	dsn, err = s.pg.DSN(dbName)
	s.Require().NoError(err)

	pgconn, err := pgx.Connect(s.ctx, dsn)
	s.Require().NoError(err)

	var v int
	row := pgconn.QueryRow(s.ctx, "SELECT 1000")
	err = row.Scan(&v)
	s.Require().NoError(err)
	s.Require().Equal(1000, v)

	err = pgconn.Close(s.ctx)
	s.Require().NoError(err)

	err = s.pg.DropDB(s.ctx, dbName)
	s.Require().NoError(err)
}

// ========================================================================
// Test suite setup
// ========================================================================
type postgresTestSuite struct {
	suite.Suite

	ctx        context.Context
	pg         PostgreSQL
	cancelFunc context.CancelFunc
}

func (s *postgresTestSuite) SetupSuite() {
	var err error
	s.pg, err = New(s.ctx)
	s.Require().NoError(err)
	s.Require().NotNil(s.pg)
}

func (s *postgresTestSuite) TearDownSuite() {
	// Dedicated timeout for cleanup
	ctx, cancel := context.WithTimeout(s.ctx, cleanupTimeout)

	defer cancel()
	defer s.cancelFunc()

	err := s.pg.Close(ctx)
	s.Require().NoError(err)
}

func TestPostgresTestSuite(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), testTimeout)
	defer cancel()

	suite.Run(t, &postgresTestSuite{
		ctx:        ctx,
		cancelFunc: cancel,
	})
}
