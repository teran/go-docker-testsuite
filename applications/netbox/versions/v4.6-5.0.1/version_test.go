package versions

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"github.com/teran/go-docker-testsuite/applications/netbox/versions"
)

const image = "index.docker.io/netboxcommunity/netbox:v4.6-5.0.1"

// Each test boots its own NetBox stack and runs DB migrations on startup, so the
// suite can take several minutes per test. The per-test timeout lives in
// versions.SetupTest; this parent context only bounds the whole run.
const suiteTimeout = 30 * time.Minute

func TestNetBoxVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), suiteTimeout)
	defer cancel()

	suite.Run(t, versions.New(ctx, image))
}
