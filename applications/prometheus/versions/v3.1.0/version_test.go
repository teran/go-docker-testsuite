package prometheus

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/teran/go-docker-testsuite/applications/prometheus/versions"
)

const image = "index.docker.io/prom/prometheus:v3.1.0"

func TestPrometheusVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	suite.Run(t, versions.New(ctx, image))
}
