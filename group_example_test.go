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

// This example demonstrates the Group API: running two containers on the same
// internal Docker network so they can reach each other by container name.
func Example_group() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	awaitRunFn := func(ctx context.Context, ht docker.HookType, c docker.Container) error {
		if ht == docker.HookTypeAfterRun {
			return c.AwaitOutput(ctx, docker.NewSubstringMatcher("running GRPC echo server"))
		}
		return nil
	}

	svr, err := docker.NewContainer(
		"my-server",
		"ghcr.io/teran/echo-grpc-server:latest",
		nil,
		docker.NewEnvironment().
			StringVar("ADDR", ":5555").
			LogLevelVar("LOG_LEVEL", logrus.TraceLevel),
		docker.NewPortBindings().
			PortDNAT(docker.ProtoTCP, 5555),
	)
	if err != nil {
		fmt.Printf("error creating server container: %v\n", err)
		return
	}

	client, err := docker.NewContainer(
		"my-client",
		"ghcr.io/teran/echo-grpc-server:latest",
		nil,
		docker.NewEnvironment().
			StringVar("ADDR", ":5555").
			LogLevelVar("LOG_LEVEL", logrus.TraceLevel),
		docker.NewPortBindings().
			PortDNAT(docker.ProtoTCP, 5555),
	)
	if err != nil {
		fmt.Printf("error creating client container: %v\n", err)
		return
	}

	g, err := docker.NewGroup("my-group",
		docker.NewApplication(svr, awaitRunFn),
		docker.NewApplication(client, awaitRunFn),
	)
	if err != nil {
		fmt.Printf("error creating group: %v\n", err)
		return
	}
	defer g.Close(ctx)

	if err := g.Run(ctx); err != nil {
		fmt.Printf("error running group: %v\n", err)
		return
	}
	fmt.Println("group started")

	// Connect to client and call server by its DNS name.
	hp, err := client.URL(docker.ProtoTCP, 5555)
	if err != nil {
		fmt.Printf("error getting client URL: %v\n", err)
		return
	}

	conn, err := grpc.NewClient(hp.String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Printf("error dialing: %v\n", err)
		return
	}
	defer conn.Close()

	cli := proto.NewRemoteEchoServiceClient(conn)
	resp, err := cli.RemoteEcho(ctx, &proto.RemoteEchoRequest{
		Remote:  "my-server:5555",
		Message: "Hello across containers!",
	})
	if err != nil {
		fmt.Printf("error calling RemoteEcho: %v\n", err)
		return
	}
	fmt.Printf("remote echo response: %s\n", resp.GetMessage())
}
