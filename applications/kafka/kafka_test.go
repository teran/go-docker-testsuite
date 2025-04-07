package kafka

import (
	"context"
	"testing"
	"time"

	"github.com/IBM/sarama"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

	"github.com/stretchr/testify/require"
)

func init() {
	log.SetLevel(log.TraceLevel)
}

func TestKafka(t *testing.T) {
	r := require.New(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)

	app, err := NewWithImage(ctx, "harbor.homelab.teran.dev/index.docker.io/apache/kafka:3.8.0")
	r.NoError(err)
	defer func() { r.NoError(app.Close(context.Background())) }()

	url, err := app.GetBrokerURL(ctx)
	r.NoError(err)

	cfg := sarama.NewConfig()
	cfg.Producer.RequiredAcks = sarama.WaitForAll
	cfg.Producer.Retry.Max = 5
	cfg.Producer.Return.Successes = true

	producer, err := sarama.NewSyncProducer([]string{url}, cfg)
	r.NoError(err)
	defer func() { _ = producer.Close() }()

	consumer, err := sarama.NewConsumer([]string{url}, cfg)
	r.NoError(err)
	defer func() { _ = consumer.Close() }()

	g, ctx := errgroup.WithContext(ctx)

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
				cancel()
			}
		}
	})

	err = g.Wait()
	r.NoError(err)
}
