package versions

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"github.com/teran/go-docker-testsuite/applications/netbox/versions"
)

const image = "index.docker.io/netboxcommunity/netbox:v4.6-5.0.1"

// The suite boots a single NetBox stack in SetupSuite and runs DB migrations
// on that one startup. This parent context only bounds the whole run, so it is
// kept generous; the per-boot timeout lives in versions.SetupSuite.
const suiteTimeout = 15 * time.Minute

func TestNetBoxVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), suiteTimeout)
	defer cancel()

	suite.Run(t, versions.New(ctx, image))
}
