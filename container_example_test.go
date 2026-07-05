package docker_test

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/teran/echo-grpc-server/presenter/proto"

	docker "github.com/teran/go-docker-testsuite"
)

// This example demonstrates using the low-level Container API: creating a
// container from a custom image, configuring environment variables and port
// bindings, waiting for a log line, and making gRPC calls.
func Example_container() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	c, err := docker.NewContainer(
		"echo-server",
		"ghcr.io/teran/echo-grpc-server:latest",
		nil,
		docker.NewEnvironment().
			StringVar("ADDR", ":5555").
			LogLevelVar("LOG_LEVEL", logrus.TraceLevel),
		docker.NewPortBindings().
			PortDNAT(docker.ProtoTCP, 5555),
	)
	if err != nil {
		fmt.Printf("error creating container: %v\n", err)
		return
	}
	defer func() { _ = c.Close(ctx) }()

	if err := c.Run(ctx); err != nil {
		fmt.Printf("error running container: %v\n", err)
		return
	}

	if err := c.AwaitOutput(ctx, docker.NewSubstringMatcher("running GRPC echo server")); err != nil {
		fmt.Printf("error waiting for server: %v\n", err)
		return
	}
	fmt.Println("server is ready")

	hp, err := c.URL(docker.ProtoTCP, 5555)
	if err != nil {
		fmt.Printf("error getting URL: %v\n", err)
		return
	}

	conn, err := grpc.NewClient(hp.String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Printf("error dialing: %v\n", err)
		return
	}
	defer func() { _ = conn.Close() }()

	cli := proto.NewEchoServiceClient(conn)
	resp, err := cli.Echo(ctx, &proto.EchoRequest{Message: "Hello!"})
	if err != nil {
		fmt.Printf("error calling Echo: %v\n", err)
		return
	}
	fmt.Printf("echo response: %s\n", resp.GetMessage())
}
