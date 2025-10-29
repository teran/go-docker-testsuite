package scylladb

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/teran/go-docker-testsuite/applications/scylladb/versions"
)

const image = "index.docker.io/scylladb/scylla:4.4.8"

func TestScyllaDBVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), versions.ScyllaDBTestDefaultTimeout)
	defer cancel()

	suite.Run(t, versions.New(ctx, image))
}
