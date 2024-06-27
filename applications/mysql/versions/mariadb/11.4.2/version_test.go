package versions

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"github.com/teran/go-docker-testsuite/applications/mysql/versions"
	"github.com/teran/go-docker-testsuite/images"
)

const image = "index.docker.io/library/mariadb:11.4.2"

func TestMySQL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	suite.Run(t, versions.New(ctx, images.ImageName(image)))
}
