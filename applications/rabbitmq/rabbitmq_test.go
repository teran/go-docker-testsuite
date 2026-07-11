package rabbitmq

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/suite"
)

func init() {
	log.SetLevel(log.TraceLevel)
}

type rabbitMQAppTestSuite struct {
	suite.Suite

	ctx    context.Context
	cancel context.CancelFunc
	app    RabbitMQ
}

func (s *rabbitMQAppTestSuite) SetupTest() {
	s.ctx, s.cancel = context.WithTimeout(s.T().Context(), 2*time.Minute)

	var err error
	s.app, err = New(s.ctx)
	s.Require().NoError(err)
}

func (s *rabbitMQAppTestSuite) TearDownTest() {
	s.Require().NoError(s.app.Close(s.T().Context()))
}

func TestRabbitMQAppTestSuite(t *testing.T) {
	suite.Run(t, &rabbitMQAppTestSuite{})
}

func (s *rabbitMQAppTestSuite) TestURLs() {
	amqpURL, err := s.app.GetAMQPURL(s.ctx)
	s.Require().NoError(err)
	s.Require().NotEmpty(amqpURL)
	s.T().Logf("AMQP URL: %s", amqpURL)

	mgmtURL, err := s.app.GetManagementURL(s.ctx)
	s.Require().NoError(err)
	s.Require().NotEmpty(mgmtURL)
	s.T().Logf("Management URL: %s", mgmtURL)
}

func (s *rabbitMQAppTestSuite) TestPublishConsume() {
	amqpURL, err := s.app.GetAMQPURL(s.ctx)
	s.Require().NoError(err)

	conn, err := amqp.Dial(amqpURL)
	s.Require().NoError(err)
	defer func() { _ = conn.Close() }()

	ch, err := conn.Channel()
	s.Require().NoError(err)
	defer func() { _ = ch.Close() }()

	q, err := ch.QueueDeclare("test-queue", false, false, false, false, nil)
	s.Require().NoError(err)

	err = ch.PublishWithContext(s.ctx, "", q.Name, false, false, amqp.Publishing{
		ContentType: "text/plain",
		Body:        []byte("hello rabbitmq"),
	})
	s.Require().NoError(err)

	msgs, err := ch.ConsumeWithContext(s.ctx, q.Name, "", true, false, false, false, nil)
	s.Require().NoError(err)

	select {
	case msg := <-msgs:
		s.Require().Equal("hello rabbitmq", string(msg.Body))
	case <-time.After(5 * time.Second):
		s.T().Fatal("timeout waiting for message")
	}
}

func (s *rabbitMQAppTestSuite) TestVHostManagement() {
	err := s.app.CreateVHost(s.ctx, "/test-vhost")
	s.Require().NoError(err)

	err = s.app.CreateUser(s.ctx, "testuser", "testpass")
	s.Require().NoError(err)

	err = s.app.SetPermissions(s.ctx, "/test-vhost", "testuser", ".*", ".*", ".*")
	s.Require().NoError(err)

	// Verify the new user can connect to the new vhost
	amqpURL, err := s.app.GetAMQPURL(s.ctx)
	s.Require().NoError(err)

	// Replace default credentials and vhost with custom ones
	connURL := fmt.Sprintf("amqp://testuser:testpass@%s/test-vhost",
		strings.TrimPrefix(amqpURL, "amqp://guest:guest@"))

	conn, err := amqp.Dial(connURL)
	s.Require().NoError(err)
	defer func() { _ = conn.Close() }()

	ch, err := conn.Channel()
	s.Require().NoError(err)
	defer func() { _ = ch.Close() }()

	_, err = ch.QueueDeclare("verify-queue", false, false, false, false, nil)
	s.Require().NoError(err)
}
