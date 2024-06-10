package scylladb

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/teran/go-docker-testsuite/applications/scylladb/versions"
	"github.com/teran/go-docker-testsuite/images"
)

const image = "index.docker.io/scylladb/scylla:5.0.12"

func TestScyllaDBVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), versions.ScyllaDBTestDefaultTimeout)
	defer cancel()

	suite.Run(t, versions.New(ctx, images.ImageName(image)))
}
