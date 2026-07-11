package rabbitmq

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/teran/go-docker-testsuite/applications/rabbitmq/versions"
)

const image = "index.docker.io/library/rabbitmq:3.13-management"

func TestRabbitMQVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()

	suite.Run(t, versions.New(ctx, image))
}
