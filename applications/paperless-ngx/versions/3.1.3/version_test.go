package versions

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"github.com/teran/go-docker-testsuite/applications/paperless-ngx/versions"
)

const image = "index.docker.io/paperlessngx/paperless-ngx:3.1.3"

// The suite boots a single Paperless-ngx stack in SetupSuite and runs DB
// migrations and model/index setup on that one startup. This parent context
// only bounds the whole run, so it is kept generous; the per-boot timeout lives
// in versions.SetupSuite.
const suiteTimeout = 15 * time.Minute

func TestPaperlessNGXVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), suiteTimeout)
	defer cancel()

	suite.Run(t, versions.New(ctx, image))
}
