package versions

import (
	"context"
	"errors"
	"time"

	"github.com/IBM/sarama"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/suite"
	"golang.org/x/sync/errgroup"

	"github.com/teran/go-docker-testsuite/applications/kafka"
)

func init() {
	log.SetLevel(log.TraceLevel)
	sarama.Logger = log.StandardLogger()
}

type KafkaTestSuite struct {
	suite.Suite

	ctx    context.Context
	cancel context.CancelFunc
	image  string

	app kafka.Kafka
}

func New(ctx context.Context, image string) *KafkaTestSuite {
	return &KafkaTestSuite{
		ctx:   ctx,
		image: image,
	}
}

func (s *KafkaTestSuite) SetupTest() {
	s.ctx, s.cancel = context.WithTimeout(s.T().Context(), time.Minute)

	var err error
	s.app, err = kafka.NewWithImage(s.ctx, s.image)
	s.Require().NoError(err)
}

func (s *KafkaTestSuite) TearDownTest() {
	if s.cancel != nil {
		s.cancel()
	}
	s.Require().NoError(s.app.Close(s.T().Context()))
}

func (s *KafkaTestSuite) TestKafkaProduceConsume() {
	url, err := s.app.GetBrokerURL(s.ctx)
	s.Require().NoError(err)

	cfg := newConfig()

	producer, err := sarama.NewSyncProducer([]string{url}, cfg)
	s.Require().NoError(err)
	defer func() { _ = producer.Close() }()

	consumer, err := sarama.NewConsumer([]string{url}, cfg)
	s.Require().NoError(err)
	defer func() { _ = consumer.Close() }()

	g, ctx := errgroup.WithContext(s.ctx)

	g.Go(func() error {
		_, _, err := producer.SendMessage(&sarama.ProducerMessage{
			Topic: "test",
			Value: sarama.StringEncoder("blah"),
		})
		return err
	})

	g.Go(func() error {
		c, err := consumer.ConsumePartition("test", 0, sarama.OffsetOldest)
		if err != nil {
			return err
		}
		defer func() { _ = c.Close() }()

		for {
			select {
			case <-ctx.Done():
				err := ctx.Err()
				if errors.Is(err, context.Canceled) {
					return nil
				}
				return err
			case err := <-c.Errors():
				return err
			case msg := <-c.Messages():
				log.WithFields(log.Fields{
					"key":   string(msg.Key),
					"value": string(msg.Value),
				}).Info("received message")
				s.cancel()
			}
		}
	})

	err = g.Wait()
	s.Require().NoError(err)
}

func newConfig() *sarama.Config {
	config := sarama.NewConfig()
	config.Version = sarama.V4_0_0_0
	config.Consumer.Offsets.AutoCommit.Enable = true
	config.Consumer.Offsets.Initial = sarama.OffsetOldest
	config.Consumer.Return.Errors = true
	config.Producer.RequiredAcks = sarama.WaitForAll
	config.Producer.Return.Successes = true
	config.Producer.Return.Errors = true
	config.Producer.Partitioner = sarama.NewRandomPartitioner
	config.ClientID = "go-docker-test-suite"

	return config
}
