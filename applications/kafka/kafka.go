package kafka

import (
	"context"
	"fmt"

	"github.com/teran/go-docker-testsuite"
	"github.com/teran/go-docker-testsuite/images"
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

func New(ctx context.Context) (Kafka, error) {
	return NewWithImage(ctx, images.Kafka)
}

func NewWithImage(ctx context.Context, image string) (Kafka, error) {
	c, err := docker.NewContainer(
		"kafka",
		image,
		nil,
		docker.NewEnvironment().
			IntVar("KAFKA_NODE_ID", 1).
			StringVar("KAFKA_PROCESS_ROLES", "broker,controller").
			Var("KAFKA_LISTENERS", func(c docker.ContainerInfo) string {
				bp, err := c.GetExternalPortMapping(docker.ProtoTCP, brokerPort)
				if err != nil {
					panic(err)
				}

				ap, err := c.GetExternalPortMapping(docker.ProtoTCP, adminPort)
				if err != nil {
					panic(err)
				}

				return fmt.Sprintf(
					"PLAINTEXT://0.0.0.0:%d,CONTROLLER://0.0.0.0:%d", bp, ap,
				)
			}).
			Var("KAFKA_ADVERTISED_LISTENERS", func(c docker.ContainerInfo) string {
				bPort, err := c.GetExternalPortMapping(docker.ProtoTCP, brokerPort)
				if err != nil {
					panic(err)
				}

				aPort, err := c.GetExternalPortMapping(docker.ProtoTCP, adminPort)
				if err != nil {
					panic(err)
				}

				ip, err := c.GetDockerHostIP()
				if err != nil {
					panic(err)
				}

				return fmt.Sprintf(
					"PLAINTEXT://%s:%d,CONTROLLER://%s:%d",
					ip, bPort, ip, aPort,
				)
			}).
			StringVar("KAFKA_CONTROLLER_LISTENER_NAMES", "CONTROLLER").
			StringVar("KAFKA_LISTENER_SECURITY_PROTOCOL_MAP", "CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT").
			Var("KAFKA_CONTROLLER_QUORUM_VOTERS", func(c docker.ContainerInfo) string {
				aPort, err := c.GetExternalPortMapping(docker.ProtoTCP, adminPort)
				if err != nil {
					panic(err)
				}

				ip, err := c.GetDockerHostIP()
				if err != nil {
					panic(err)
				}
				return fmt.Sprintf("1@%s:%d", ip, aPort)
			}),
		docker.NewDirectPortBinding().
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
