package docker

import (
	"context"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/teran/echo-grpc-server/presenter/proto"
	"github.com/teran/go-docker-testsuite/images"
)

func init() {
	log.SetLevel(log.TraceLevel)
}

func TestGroup(t *testing.T) {
	r := require.New(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	awaitRunFn := func(ctx context.Context, ht HookType, c Container) error {
		if ht == HookTypeAfterRun {
			return c.AwaitOutput(ctx, NewSubstringMatcher("running GRPC echo server"))
		}
		return nil
	}

	apps := []*Application{}
	c1, err := NewContainer(
		"server",
		images.EchoServer,
		nil,
		NewEnvironment().
			StringVar("ADDR", ":5555").
			LogLevelVar("LOG_LEVEL", log.TraceLevel),
		NewPortBindings().
			PortDNAT(ProtoTCP, 5555),
	)
	r.NoError(err)

	apps = append(apps, NewApplication(c1, awaitRunFn))

	c2, err := NewContainer(
		"client",
		images.EchoServer,
		nil,
		NewEnvironment().
			StringVar("ADDR", ":5555").
			LogLevelVar("LOG_LEVEL", log.TraceLevel),
		NewPortBindings().
			PortDNAT(ProtoTCP, 5555),
	)
	r.NoError(err)
	apps = append(apps, NewApplication(c2, awaitRunFn))

	g, err := NewGroup("test-group", apps...)
	r.NoError(err)

	defer func() { _ = g.Close(ctx) }()

	err = g.Run(ctx)
	r.NoError(err)

	cl, err := c2.URL(ProtoTCP, 5555)
	r.NoError(err)

	dial, err := grpc.NewClient(cl.String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	r.NoError(err)

	cli := proto.NewRemoteEchoServiceClient(dial)
	resp, err := cli.RemoteEcho(ctx, &proto.RemoteEchoRequest{
		Remote:  "server:5555",
		Message: "test message",
	})
	r.NoError(err)
	r.Equal("test message", resp.GetMessage())

	err = g.Close(ctx)
	r.NoError(err)
}
