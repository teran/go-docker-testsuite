package kafka

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/IBM/sarama"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

	"github.com/stretchr/testify/suite"
)

func init() {
	log.SetLevel(log.TraceLevel)
	sarama.Logger = log.StandardLogger()
}

func (s *kafkaAppTestSuite) TestKafkaConsumer() {
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
				if err == context.Canceled {
					return nil
				}
				return err
			case err := <-c.Errors():
				return err
			case msg := <-c.Messages():
				log.Infof("Received messages: key=%s; value=%s", string(msg.Key), string(msg.Value))
				s.cancel()
			}
		}
	})

	err = g.Wait()
	s.Require().NoError(err)
}

func (s *kafkaAppTestSuite) TestKafkaConsumerGroup() {
	url, err := s.app.GetBrokerURL(s.ctx)
	s.Require().NoError(err)

	cfg := newConfig()

	producer, err := sarama.NewSyncProducer([]string{url}, cfg)
	s.Require().NoError(err)
	defer func() { _ = producer.Close() }()

	client, err := sarama.NewConsumerGroup([]string{url}, "test-group", cfg)
	if err != nil {
		log.Panicf("Error creating consumer group client: %v", err)
	}

	g, ctx := errgroup.WithContext(s.ctx)

	consumer := &cg{
		cancelFn: s.cancel,
	}

	g.Go(func() error {
		return client.Consume(ctx, []string{"test"}, consumer)
	})

	g.Go(func() error {
		partition, offset, err := producer.SendMessage(&sarama.ProducerMessage{
			Topic: "test",
			Value: sarama.StringEncoder("blah"),
		})
		log.WithFields(log.Fields{
			"partition": partition,
			"offset":    offset,
			"topic":     "test",
		}).Debug("message produced")

		return err
	})

	err = g.Wait()
	s.Require().NoError(err)
}

// Definitions ...
type kafkaAppTestSuite struct {
	suite.Suite

	ctx    context.Context
	cancel context.CancelFunc
	app    Kafka
}

func (s *kafkaAppTestSuite) SetupTest() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), time.Minute)

	var err error
	s.app, err = New(s.ctx)
	s.Require().NoError(err)
}

func (s *kafkaAppTestSuite) TearDownTest() {
	s.Require().NoError(s.app.Close(context.TODO()))
}

func TestKafkaAppTestSuite(t *testing.T) {
	suite.Run(t, &kafkaAppTestSuite{})
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

type cg struct {
	cancelFn context.CancelFunc
}

func (c *cg) Setup(sarama.ConsumerGroupSession) error {
	return nil
}

func (c *cg) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

func (c *cg) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case <-session.Context().Done():
			return session.Context().Err()
		case message, ok := <-claim.Messages():
			if !ok {
				log.Printf("message channel was closed")
				return errors.New("message channel was closed")
			}
			log.WithFields(log.Fields{
				"value":     string(message.Value),
				"timestamp": message.Timestamp.Format(time.RFC3339),
				"topic":     message.Topic,
			}).Debug("Message claimed")

			session.MarkMessage(message, "")
			c.cancelFn()
		}
	}
}
