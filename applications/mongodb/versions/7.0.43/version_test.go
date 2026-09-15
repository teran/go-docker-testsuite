package versions

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"github.com/teran/go-docker-testsuite/applications/mongodb/versions"
)

const image = "index.docker.io/library/mongo:7.0.43"

func TestMongoDBVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	suite.Run(t, versions.New(ctx, image))
}
