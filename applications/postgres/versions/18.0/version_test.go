package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/teran/go-docker-testsuite/v2/applications/postgres/versions"
)

const image = "index.docker.io/library/postgres:18.0"

func TestPostgreSQLVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()

	suite.Run(t, versions.New(ctx, image))
}
