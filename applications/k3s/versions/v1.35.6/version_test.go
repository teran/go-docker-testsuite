package k3s

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/teran/go-docker-testsuite/applications/k3s/versions"
)

const image = "index.docker.io/rancher/k3s:v1.35.6-k3s1"

func TestK3sVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	suite.Run(t, versions.New(ctx, image))
}
