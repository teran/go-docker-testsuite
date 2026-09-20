package frr

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/teran/go-docker-testsuite/applications/frr/versions"
)

const image = "quay.io/frrouting/frr:10.7.1"

func TestFRRVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()

	suite.Run(t, versions.New(ctx, image))
}
