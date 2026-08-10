package opensearch

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"github.com/teran/go-docker-testsuite/applications/opensearch/versions"
)

const image = "index.docker.io/opensearchproject/opensearch:2.17.1"

func TestOpenSearchVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	suite.Run(t, versions.New(ctx, image))
}
