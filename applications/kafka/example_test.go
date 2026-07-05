package kafka_test

import (
	"context"
	"fmt"
	"time"

	"github.com/IBM/sarama"

	"github.com/teran/go-docker-testsuite/applications/kafka"
)

// This example demonstrates starting a Kafka container, producing a message
// and consuming it back using the Sarama library.
func Example() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	app, err := kafka.New(ctx)
	if err != nil {
		fmt.Printf("error: %v (is Docker running?)\n", err)
		return
	}
	defer app.Close(ctx)

	brokerURL, err := app.GetBrokerURL(ctx)
	if err != nil {
		fmt.Printf("error getting broker URL: %v\n", err)
		return
	}
	fmt.Println("kafka broker:", brokerURL)

	cfg := sarama.NewConfig()
	cfg.Version = sarama.V4_0_0_0
	cfg.Producer.Return.Successes = true
	cfg.Consumer.Return.Errors = true

	producer, err := sarama.NewSyncProducer([]string{brokerURL}, cfg)
	if err != nil {
		fmt.Printf("error creating producer: %v\n", err)
		return
	}
	defer producer.Close()

	partition, offset, err := producer.SendMessage(&sarama.ProducerMessage{
		Topic: "events",
		Value: sarama.StringEncoder("hello"),
	})
	if err != nil {
		fmt.Printf("error producing: %v\n", err)
		return
	}
	fmt.Printf("message produced [p=%d, o=%d]\n", partition, offset)

	consumer, err := sarama.NewConsumer([]string{brokerURL}, cfg)
	if err != nil {
		fmt.Printf("error creating consumer: %v\n", err)
		return
	}
	defer consumer.Close()

	partConsumer, err := consumer.ConsumePartition("events", 0, sarama.OffsetNewest)
	if err != nil {
		fmt.Printf("error consuming partition: %v\n", err)
		return
	}
	defer partConsumer.Close()

	select {
	case msg := <-partConsumer.Messages():
		fmt.Printf("message received: %s\n", string(msg.Value))
	case <-ctx.Done():
		fmt.Printf("timeout: %v\n", ctx.Err())
	}
}
