package versions

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"github.com/teran/go-docker-testsuite/v2/applications/mysql/versions"
)

const image = "index.docker.io/library/mariadb:12.0.2"

func TestMySQL(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	suite.Run(t, versions.New(ctx, image))
}
