package ceph

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/teran/go-docker-testsuite/applications/ceph/versions"
)

const image = "ghcr.io/teran/ceph-container/ceph:v19.2.4"

func TestCephVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()

	suite.Run(t, versions.New(ctx, image))
}
