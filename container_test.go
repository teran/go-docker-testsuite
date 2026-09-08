package docker

import (
	"context"
	"testing"
	"time"

	dockerContainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/strslice"
	"github.com/docker/docker/client"
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

func TestWithMemoryLimit(t *testing.T) {
	r := require.New(t)

	hc := &dockerContainer.HostConfig{}
	WithMemoryLimit(512 * 1024 * 1024)(hc)
	r.Equal(int64(512*1024*1024), hc.Memory)
}

func TestWithMemoryReservation(t *testing.T) {
	r := require.New(t)

	hc := &dockerContainer.HostConfig{}
	WithMemoryReservation(256 * 1024 * 1024)(hc)
	r.Equal(int64(256*1024*1024), hc.MemoryReservation)
}

func TestWithMemorySwap(t *testing.T) {
	r := require.New(t)

	hc := &dockerContainer.HostConfig{}
	WithMemorySwap(1024 * 1024 * 1024)(hc)
	r.Equal(int64(1024*1024*1024), hc.MemorySwap)

	// -1 enables unlimited swap.
	hc2 := &dockerContainer.HostConfig{}
	WithMemorySwap(-1)(hc2)
	r.Equal(int64(-1), hc2.MemorySwap)
}

func TestWithCPUs(t *testing.T) {
	r := require.New(t)

	hc := &dockerContainer.HostConfig{}
	WithCPUs(0.5)(hc)
	r.Equal(int64(500000000), hc.NanoCPUs)

	// Non-positive values are no-ops: the field must be left untouched.
	const unchanged = int64(123456789)
	for _, count := range []float64{0, -1, -0.5} {
		hc := &dockerContainer.HostConfig{}
		hc.NanoCPUs = unchanged
		WithCPUs(count)(hc)
		r.Equal(unchanged, hc.NanoCPUs, "WithCPUs(%v) must be a no-op", count)
	}
}

func TestWithCpusetCpus(t *testing.T) {
	r := require.New(t)

	hc := &dockerContainer.HostConfig{}
	WithCpusetCpus("0-2,4")(hc)
	r.Equal("0-2,4", hc.CpusetCpus)
}

func TestWithPidsLimit(t *testing.T) {
	r := require.New(t)

	hc := &dockerContainer.HostConfig{}
	WithPidsLimit(256)(hc)
	r.NotNil(hc.PidsLimit)
	r.Equal(int64(256), *hc.PidsLimit)
}

func TestParseRAMSize(t *testing.T) {
	testCases := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		{
			name:  "512m",
			input: "512m",
			want:  512 * 1024 * 1024,
		},
		{
			name:  "1g",
			input: "1g",
			want:  1024 * 1024 * 1024,
		},
		{
			name:  "1.5g",
			input: "1.5g",
			want:  1610612736,
		},
		{
			name:  "1kb",
			input: "1kb",
			want:  1024,
		},
		{
			name:  "zero",
			input: "0",
			want:  0,
		},
		{
			name:    "negative",
			input:   "-1g",
			wantErr: true,
		},
		{
			name:    "garbage",
			input:   "garbage",
			wantErr: true,
		},
		{
			name:    "empty",
			input:   "",
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := require.New(t)

			got, err := ParseRAMSize(tc.input)
			if tc.wantErr {
				r.Error(err)
				return
			}

			r.NoError(err)
			r.Equal(tc.want, got)
		})
	}
}

func TestContainerResourceLimits(t *testing.T) {
	r := require.New(t)

	if testing.Short() {
		t.Skip("skipping resource limits integration test in -short mode")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	r.NoError(err)
	if _, err := cli.Ping(ctx); err != nil {
		t.Skipf("skipping resource limits test: docker daemon unavailable: %v", err)
	}

	const memBytes = int64(128 * 1024 * 1024) // 128MiB

	c, err := NewContainerWithClient(
		cli,
		"resource-limits-test",
		"index.docker.io/library/busybox:latest",
		[]string{"sleep", "300"},
		NewEnvironment(),
		NewPortBindings(),
		WithMemoryLimit(memBytes),
		WithMemoryReservation(memBytes/2),
		WithCPUs(0.5),
		WithPidsLimit(256),
	)
	r.NoError(err)

	err = c.Run(ctx)
	r.NoError(err)
	defer func() { _ = c.Close(ctx) }()

	// Inspect the running container and verify the limits were applied to the
	// actual HostConfig (as opposed to merely being passed to the API).
	insp, err := cli.ContainerInspect(ctx, c.ID())
	r.NoError(err)

	r.Equal(memBytes, insp.HostConfig.Memory)
	r.Equal(memBytes/2, insp.HostConfig.MemoryReservation)
	r.Equal(int64(500000000), insp.HostConfig.NanoCPUs)
	r.NotNil(insp.HostConfig.PidsLimit)
	r.Equal(int64(256), *insp.HostConfig.PidsLimit)
}
