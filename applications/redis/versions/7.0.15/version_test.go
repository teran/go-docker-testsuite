package versions

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"github.com/teran/go-docker-testsuite/v2/applications/redis/versions"
)

const image = "index.docker.io/library/redis:7.0.15"

func TestRedisVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	suite.Run(t, versions.New(ctx, image))
}
