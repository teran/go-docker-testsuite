package kafka

import (
	"context"

	"github.com/teran/go-docker-testsuite"
)

const (
	brokerPort = 9092
	adminPort  = 9093
)

type Kafka interface {
	Close(context.Context) error
	GetBrokerURL(ctx context.Context) (string, error)
	GetAdminURL(ctx context.Context) (string, error)
}

type kafka struct {
	c docker.Container
}

func NewWithImage(ctx context.Context, image string) (Kafka, error) {
	c, err := docker.NewContainer(
		"kafka",
		"apache/kafka:3.9.0",
		nil,
		docker.NewEnvironment().
			IntVar("KAFKA_NODE_ID", 1).
			StringVar("KAFKA_PROCESS_ROLES", "broker,controller").
			StringVar("KAFKA_LISTENERS", "PLAINTEXT://0.0.0.0:9092,CONTROLLER://0.0.0.0:9093").
			StringVar("KAFKA_ADVERTISED_LISTENERS", "PLAINTEXT://localhost:9092").
			StringVar("KAFKA_CONTROLLER_LISTENER_NAMES", "CONTROLLER").
			StringVar("KAFKA_LISTENER_SECURITY_PROTOCOL_MAP", "CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT").
			StringVar("KAFKA_CONTROLLER_QUORUM_VOTERS", "1@localhost:9093"),
		docker.NewPortBindings().
			PortDNAT(docker.ProtoTCP, brokerPort).
			PortDNAT(docker.ProtoTCP, adminPort),
	)
	if err != nil {
		return nil, err
	}

	err = c.Run(ctx)
	if err != nil {
		return nil, err
	}

	err = c.AwaitOutput(ctx, docker.NewSubstringMatcher("] Kafka Server started ("))
	if err != nil {
		return nil, err
	}

	return &kafka{
		c: c,
	}, nil
}

func (k *kafka) GetBrokerURL(ctx context.Context) (string, error) {
	hp, err := k.c.URL(docker.ProtoTCP, brokerPort)
	if err != nil {
		return "", err
	}

	return hp.String(), nil
}

func (k *kafka) GetAdminURL(ctx context.Context) (string, error) {
	hp, err := k.c.URL(docker.ProtoTCP, adminPort)
	if err != nil {
		return "", err
	}

	return hp.String(), nil
}

func (k *kafka) Close(ctx context.Context) error {
	return k.c.Close(ctx)
}
