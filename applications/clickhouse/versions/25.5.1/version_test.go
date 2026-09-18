package clickhouse

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/teran/go-docker-testsuite/applications/clickhouse/versions"
)

const image = "index.docker.io/clickhouse/clickhouse-server:25.5.1"

func TestClickHouseVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	suite.Run(t, versions.New(ctx, image))
}
