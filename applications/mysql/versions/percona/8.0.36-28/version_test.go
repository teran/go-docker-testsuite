package versions

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"github.com/teran/go-docker-testsuite/applications/mysql/versions"
)

const image = "index.docker.io/library/percona:8.0.36-28"

func TestMySQL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	suite.Run(t, versions.New(ctx, image))
}
