package versions

import (
	"context"
	"fmt"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/suite"

	"github.com/teran/go-docker-testsuite/applications/rabbitmq"
)

func init() {
	log.SetLevel(log.TraceLevel)
}

type testSuite struct {
	suite.Suite

	ctx   context.Context
	image string
}

func New(ctx context.Context, image string) *testSuite {
	return &testSuite{
		ctx:   ctx,
		image: image,
	}
}

func (s *testSuite) TestAMQP() {
	app, err := rabbitmq.NewWithImage(s.ctx, s.image)
	s.Require().NoError(err)
	defer func() {
		err := app.Close(s.ctx)
		s.Require().NoError(err)
	}()

	amqpURL, err := app.GetAMQPURL(s.ctx)
	s.Require().NoError(err)

	conn, err := amqp.Dial(amqpURL)
	s.Require().NoError(err)
	defer func() { _ = conn.Close() }()

	ch, err := conn.Channel()
	s.Require().NoError(err)
	defer func() { _ = ch.Close() }()

	q, err := ch.QueueDeclare("version-test-queue", false, false, false, false, nil)
	s.Require().NoError(err)

	err = ch.PublishWithContext(s.ctx, "", q.Name, false, false, amqp.Publishing{
		ContentType: "text/plain",
		Body:        []byte("version check"),
	})
	s.Require().NoError(err)

	msgs, err := ch.ConsumeWithContext(s.ctx, q.Name, "", true, false, false, false, nil)
	s.Require().NoError(err)

	select {
	case msg := <-msgs:
		s.Require().Equal("version check", string(msg.Body))
	case <-time.After(5 * time.Second):
		s.T().Fatal("timeout waiting for message")
	}
}

func (s *testSuite) TestManagementAPI() {
	app, err := rabbitmq.NewWithImage(s.ctx, s.image)
	s.Require().NoError(err)
	defer func() {
		err := app.Close(s.ctx)
		s.Require().NoError(err)
	}()

	err = app.CreateVHost(s.ctx, "/version-test-vhost")
	s.Require().NoError(err)

	err = app.CreateUser(s.ctx, "version-user", "version-pass")
	s.Require().NoError(err)

	err = app.SetPermissions(s.ctx, "/version-test-vhost", "version-user", ".*", ".*", ".*")
	s.Require().NoError(err)

	// Verify by connecting with the new credentials
	amqpURL, err := app.GetAMQPURL(s.ctx)
	s.Require().NoError(err)

	connURL := fmt.Sprintf("amqp://version-user:version-pass@%s/version-test-vhost",
		strings.TrimPrefix(amqpURL, "amqp://guest:guest@"))

	conn, err := amqp.Dial(connURL)
	s.Require().NoError(err)
	_ = conn.Close()
}
