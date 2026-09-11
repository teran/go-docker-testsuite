package nginx

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/teran/go-docker-testsuite/applications/nginx/versions"
)

const image = "index.docker.io/library/nginx:1.27-alpine"

func TestNginxVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()

	suite.Run(t, versions.New(ctx, image))
}
