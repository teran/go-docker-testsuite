package docker

import (
	"context"
	"testing"
	"time"

	dockerContainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/strslice"
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

func TestImagePrefix(t *testing.T) {
	r := require.New(t)

	t.Setenv("IMAGE_PREFIX", "test-prefix")
	c, err := NewContainer("test", "image:test", []string{}, NewEnvironment(), NewPortBindings())
	r.NoError(err)
	r.Equal("test-prefix/image:test", c.(*container).image)
}

func TestWithDevices(t *testing.T) {
	r := require.New(t)

	// The option must not require a running Docker daemon, so exercise it
	// through NewHostConfig with an empty PortBindings.
	hc, err := NewHostConfig(NewPortBindings(), WithDevices(
		"/dev/kvm",
		"/dev/net/tun:/dev/net/tun",
		"/dev/foo:/dev/bar:rw",
	))
	r.NoError(err)

	r.Equal([]dockerContainer.DeviceMapping{
		{
			PathOnHost:        "/dev/kvm",
			PathInContainer:   "/dev/kvm",
			CgroupPermissions: "rwm",
		},
		{
			PathOnHost:        "/dev/net/tun",
			PathInContainer:   "/dev/net/tun",
			CgroupPermissions: "rwm",
		},
		{
			PathOnHost:        "/dev/foo",
			PathInContainer:   "/dev/bar",
			CgroupPermissions: "rw",
		},
	}, hc.Devices)
}

func TestWithCapabilities(t *testing.T) {
	r := require.New(t)

	hc, err := NewHostConfig(NewPortBindings(),
		WithCapDrop("ALL"),
		WithCapAdd("NET_ADMIN", "SYS_NICE"),
		WithSecurityOpt("seccomp=unconfined"),
	)
	r.NoError(err)

	r.Equal(strslice.StrSlice{"ALL"}, hc.CapDrop)
	r.Equal(strslice.StrSlice{"NET_ADMIN", "SYS_NICE"}, hc.CapAdd)
	r.Equal([]string{"seccomp=unconfined"}, hc.SecurityOpt)
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

	err = c.Ping(ctx)
	r.NoError(err)

	err = c.Run(ctx)
	r.NoError(err)

	defer func() { _ = c.Close(ctx) }()

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
