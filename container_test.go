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

func TestContainerURLPortNotRegistered(t *testing.T) {
	r := require.New(t)

	t.Setenv("DOCKER_HOST", "tcp://127.0.0.1:2375")

	pb := NewPortBindings().
		PortDNAT(ProtoTCP, 5555)
	c, err := NewContainer("test-url-notfound", "test:image", nil, NewEnvironment(), pb)
	r.NoError(err)

	_, err = c.URL(ProtoTCP, 9999)
	r.Error(err)
	r.Contains(err.Error(), "port `9999/tcp` is not registered")
}

func TestContainerURLEmptyHostPort(t *testing.T) {
	r := require.New(t)

	c := &container{
		ports: &PortBindings{
			portBindings: map[string][]Binding{
				"1234/tcp": {{HostIP: "127.0.0.1", HostPort: ""}},
			},
		},
	}

	_, err := c.URL(ProtoTCP, 1234)
	r.Error(err)
	r.Contains(err.Error(), "external port is not defined for `1234`")
}

func TestContainerURLUnexpectedPorts(t *testing.T) {
	r := require.New(t)

	c := &container{
		ports: &PortBindings{
			portBindings: map[string][]Binding{
				"1234/tcp": {
					{HostIP: "127.0.0.1", HostPort: "1234"},
					{HostIP: "127.0.0.1", HostPort: "5678"},
				},
			},
		},
	}

	_, err := c.URL(ProtoTCP, 1234)
	r.Error(err)
	r.Contains(err.Error(), "unexpected amount of ports returned by name")
}

func TestImagePrefix(t *testing.T) {
	r := require.New(t)

	t.Setenv("IMAGE_PREFIX", "test-prefix")
	c, err := NewContainer("test", "image:test", []string{}, NewEnvironment(), NewPortBindings())
	r.NoError(err)
	r.Equal("test-prefix/image:test", c.(*container).image)
}

func TestContainerRun(t *testing.T) {
	r := require.New(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	c, err := NewContainer(
		"test-container",
		images.EchoServer,
		nil,
		NewEnvironment().
			StringVar("ADDR", ":5555").
			LogLevelVar("LOG_LEVEL", log.TraceLevel),
		NewPortBindings().
			PortDNAT(ProtoTCP, 5555),
	)
	r.NoError(err)

	defer func() { _ = c.Close(ctx) }()

	err = c.Ping(ctx)
	r.NoError(err)

	err = c.Run(ctx)
	r.NoError(err)

	err = c.AwaitOutput(ctx, NewSubstringMatcher("running GRPC echo server"))
	r.NoError(err)

	lines, err := c.GetOutput(ctx, NewSubstringMatcher(""))
	r.NoError(err)
	r.NotEmpty(lines)
	r.Contains(lines[0], "running GRPC echo server")

	defer func() { _ = c.Close(ctx) }()

	hp, err := c.URL(ProtoTCP, 5555)
	r.NoError(err)

	dial, err := grpc.NewClient(hp.String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	r.NoError(err)

	cli := proto.NewEchoServiceClient(dial)

	resp, err := cli.Echo(ctx, &proto.EchoRequest{
		Message: "test message",
	})
	r.NoError(err)
	r.Equal("test message", resp.GetMessage())

	err = c.AwaitOutput(ctx, NewSubstringMatcher(`test message`))
	r.NoError(err)

	resp, err = cli.Echo(ctx, &proto.EchoRequest{
		Message: "some another message",
	})
	r.NoError(err)
	r.Equal("some another message", resp.GetMessage())
}
