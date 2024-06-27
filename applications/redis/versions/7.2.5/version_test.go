package versions

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"github.com/teran/go-docker-testsuite/applications/redis/versions"
	"github.com/teran/go-docker-testsuite/images"
)

const image = "index.docker.io/library/redis:7.2.5"

func TestRedisVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	suite.Run(t, versions.New(ctx, images.ImageName(image)))
}
