package rabbitmq_test

import (
	"context"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/teran/go-docker-testsuite/applications/rabbitmq"
)

// This example demonstrates starting a RabbitMQ container, producing a message
// and consuming it back using the AMQP 0-9-1 protocol.
func Example() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	app, err := rabbitmq.New(ctx)
	if err != nil {
		fmt.Printf("error: %v (is Docker running?)\n", err)
		return
	}
	defer func() { _ = app.Close(ctx) }()

	amqpURL, err := app.GetAMQPURL(ctx)
	if err != nil {
		fmt.Printf("error getting AMQP URL: %v\n", err)
		return
	}
	fmt.Println("rabbitmq amqp:", amqpURL)

	conn, err := amqp.Dial(amqpURL)
	if err != nil {
		fmt.Printf("error connecting: %v\n", err)
		return
	}
	defer func() { _ = conn.Close() }()

	ch, err := conn.Channel()
	if err != nil {
		fmt.Printf("error creating channel: %v\n", err)
		return
	}
	defer func() { _ = ch.Close() }()

	q, err := ch.QueueDeclare("greetings", false, false, false, false, nil)
	if err != nil {
		fmt.Printf("error declaring queue: %v\n", err)
		return
	}

	err = ch.PublishWithContext(ctx, "", q.Name, false, false, amqp.Publishing{
		ContentType: "text/plain",
		Body:        []byte("hello rabbitmq"),
	})
	if err != nil {
		fmt.Printf("error publishing: %v\n", err)
		return
	}
	fmt.Println("message produced")

	msgs, err := ch.ConsumeWithContext(ctx, q.Name, "", true, false, false, false, nil)
	if err != nil {
		fmt.Printf("error consuming: %v\n", err)
		return
	}

	select {
	case msg := <-msgs:
		fmt.Printf("message received: %s\n", string(msg.Body))
	case <-time.After(5 * time.Second):
		fmt.Println("timeout waiting for message")
	}
}
