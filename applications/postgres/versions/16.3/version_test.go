package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/teran/go-docker-testsuite/applications/postgres/versions"
)

const image = "postgres:16.3"

func TestScyllaDBVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	suite.Run(t, versions.New(ctx, image))
}
