package versions

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"github.com/teran/go-docker-testsuite/applications/forgejo/versions"
)

const image = "codeberg.org/forgejo/forgejo:16.0.4"

func TestForgejoVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	suite.Run(t, versions.New(ctx, image))
}
