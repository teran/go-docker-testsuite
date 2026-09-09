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

// This example demonstrates running a command inside a running container via
// Container.Exec: capturing stdout, stderr and the exit code, and checking
// that the command succeeded with ExecResult.Error().
func ExampleContainer_Exec() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	c, err := docker.NewContainer(
		"exec-example",
		"busybox:latest",
		[]string{"sleep", "300"},
		nil,
		nil,
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

	res, err := c.Exec(ctx, []string{"echo", "hello"})
	if err != nil {
		fmt.Printf("error executing command: %v\n", err)
		return
	}

	if err := res.Error(); err != nil {
		fmt.Printf("command failed: %v\n", err)
		return
	}

	fmt.Printf("exit code: %d\n", res.ExitCode)
	fmt.Printf("output: %s", res.Stdout)
}

// This example demonstrates NewContainerWithLifecycle: running a startup
// command right after the container starts and an after-ready command once a
// readiness log line is observed.
func ExampleNewContainerWithLifecycle() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	c, err := docker.NewContainerWithLifecycle(
		"lifecycle-example",
		"busybox:latest",
		[]string{"sh", "-c", "echo READY; sleep 300"},
		nil,
		nil,
		docker.WithStartupCommand("sh", "-c", "echo startup > /tmp/startup.txt"),
		docker.WithAfterReadyCommand(
			docker.NewSubstringMatcher("READY"),
			"sh", "-c", "echo seeded > /tmp/seeded.txt",
		),
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

	startup, err := c.Exec(ctx, []string{"cat", "/tmp/startup.txt"})
	if err != nil {
		fmt.Printf("error reading startup marker: %v\n", err)
		return
	}

	seeded, err := c.Exec(ctx, []string{"cat", "/tmp/seeded.txt"})
	if err != nil {
		fmt.Printf("error reading seeded marker: %v\n", err)
		return
	}

	fmt.Printf("startup: %s", startup.Stdout)
	fmt.Printf("seeded: %s", seeded.Stdout)
}
