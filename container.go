package docker

import (
	"bufio"
	"context"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	dockerContainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/teran/go-docker-testsuite/internal/ptr"
)

const (
	defaultStopTimeout = 1 * time.Minute
)

var errImageIsNotPulled = errors.New("image is not pulled")

type (
	ContainerID = string
	NetworkID   = string
)

// ContainerOption modifies the docker HostConfig before container creation.
type ContainerOption func(*dockerContainer.HostConfig)

// WithPrivileged grants the container elevated privileges.
func WithPrivileged() ContainerOption {
	return func(hc *dockerContainer.HostConfig) {
		hc.Privileged = true
	}
}

// WithTmpfs mounts tmpfs filesystems at the given paths.
func WithTmpfs(m map[string]string) ContainerOption {
	return func(hc *dockerContainer.HostConfig) {
		hc.Tmpfs = m
	}
}

// WithBinds adds volume bind mounts (host:container[:mode]).
func WithBinds(binds ...string) ContainerOption {
	return func(hc *dockerContainer.HostConfig) {
		hc.Binds = binds
	}
}

// Container exposes interface to control the container runtime
type Container interface {
	AwaitOutput(ctx context.Context, m Matcher) error
	GetOutput(ctx context.Context, m ...Matcher) ([]string, error)
	Close(ctx context.Context) error
	ID() ContainerID
	Name() string
	NetworkAttach(networkID string) error
	Ping(ctx context.Context) error
	Run(ctx context.Context) error
	URL(proto Protocol, port uint16) (*HostPort, error)
}

type container struct {
	cli *client.Client

	name          string
	image         string
	env           Environment
	cmd           []string
	containerID   ContainerID
	networkID     NetworkID
	ports         *PortBindings
	indirectPorts map[string]string
	containerOpts []ContainerOption
}

// New creates new container instance from remote docker image
func NewContainer(name, image string, cmd []string, environment Environment, ports *PortBindings, opts ...ContainerOption) (Container, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}

	return NewContainerWithClient(cli, name, image, cmd, environment, ports, opts...)
}

// NewContainerWithClient creates new container from remote docker image and allows
// to pass custom docker.Client instance
func NewContainerWithClient(cli *client.Client, name, image string, cmd []string, env Environment, ports *PortBindings, opts ...ContainerOption) (Container, error) {
	log.WithFields(log.Fields{
		"name":  name,
		"image": image,
	}).Debugf("initializing container")

	imageRef := image
	prefix := os.Getenv("IMAGE_PREFIX")
	if prefix != "" {
		imageRef = strings.TrimRight(prefix, "/") + "/" + strings.TrimLeft(imageRef, "/")
		log.WithFields(log.Fields{
			"original": image,
			"prefixed": imageRef,
		}).Trace("Setting prefix for image (for proxy purposes since IMAGE_PREFIX is present)")
	}

	return &container{
		cli:           cli,
		name:          name,
		image:         imageRef,
		cmd:           cmd,
		env:           env,
		ports:         ports,
		indirectPorts: make(map[string]string),
		containerOpts: opts,
	}, nil
}

// AwaitOutput blocks the execution for any of (whatever comes first): string matched Matcher or timeout
func (c *container) AwaitOutput(ctx context.Context, m Matcher) error {
	rd, err := c.cli.ContainerLogs(ctx, c.containerID, dockerContainer.LogsOptions{
		ShowStderr: true,
		ShowStdout: true,
		Follow:     true,
	})
	if err != nil {
		return err
	}
	defer func() { _ = rd.Close() }()

	s := bufio.NewScanner(rd)
	for s.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			l := s.Text()

			log.WithFields(log.Fields{
				"line": l,
			}).Tracef("processing log line")

			if m(l) {
				return nil
			}
		}
	}

	return s.Err()
}

func (c *container) GetOutput(ctx context.Context, ms ...Matcher) ([]string, error) {
	rd, err := c.cli.ContainerLogs(ctx, c.containerID, dockerContainer.LogsOptions{
		ShowStderr: true,
		ShowStdout: true,
		Follow:     false,
	})
	if err != nil {
		return nil, err
	}
	defer func() { _ = rd.Close() }()

	out := []string{}
	s := bufio.NewScanner(rd)
	for s.Scan() {
		l := s.Text()

		log.WithFields(log.Fields{
			"line": l,
		}).Tracef("processing log line")

		for _, m := range ms {
			if m(l) {
				out = append(out, l)
			}
		}
	}

	return out, s.Err()
}

func (c *container) Name() string {
	return c.name
}

// ID returns the Docker container ID. It is only available after Run() is called.
func (c *container) ID() ContainerID {
	return c.containerID
}

// NetworkAttach attaches the container to the specific network
// Usable when it's needed to group amount of containers with IP-level connectivity
func (c *container) NetworkAttach(networkID string) error {
	c.networkID = networkID
	return nil
}

// Ping gonna ping (the Docker daemon)
func (c *container) Ping(ctx context.Context) error {
	_, err := c.cli.Ping(ctx)
	return err
}

// Run starts the container
func (c *container) Run(ctx context.Context) error {
	err := c.pullImage(ctx)
	if err != nil {
		return err
	}

	containerConfig := &dockerContainer.Config{
		Image:        c.image,
		Env:          c.env.Eval(newContainerInfoFromContainer(c)),
		Cmd:          c.cmd,
		ExposedPorts: c.ports.portSet(),
		Labels: map[string]string{
			"go-docker-testsuite.name": c.name,
		},
	}

	networkConfig := &network.NetworkingConfig{}

	log.WithFields(log.Fields{
		"ports": c.ports,
	}).Trace("creating new host config ...")

	hostConfig, err := NewHostConfig(c.ports, c.containerOpts...)
	if err != nil {
		return errors.Wrap(err, "error gathering host configuration")
	}

	container, err := c.cli.ContainerCreate(
		ctx,
		containerConfig,
		hostConfig,
		networkConfig,
		nil,
		"",
	)
	if err != nil {
		return errors.Wrap(err, "error creating container")
	}

	c.containerID = container.ID

	if c.networkID != "" {
		err := c.cli.NetworkConnect(ctx, c.networkID, c.containerID, &network.EndpointSettings{
			Aliases: []string{c.name},
		})
		if err != nil {
			return err
		}
	}

	err = c.cli.ContainerStart(ctx, c.containerID, dockerContainer.StartOptions{})
	return errors.Wrap(err, "error starting container")
}

// Close cleans up the env (stops & removes the container)
func (c *container) Close(ctx context.Context) error {
	if c.containerID == "" {
		return nil
	}

	// If the provided context is already expired, use a fresh one for cleanup
	// so a test timeout doesn't prevent container removal.
	if dl, ok := ctx.Deadline(); ok && time.Until(dl) <= 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), defaultStopTimeout)
		defer cancel()
	}

	timeout := defaultStopTimeout
	if dl, ok := ctx.Deadline(); ok {
		if remaining := time.Until(dl); remaining > 0 {
			timeout = remaining
		}
	}

	err := c.cli.ContainerStop(ctx, c.containerID, dockerContainer.StopOptions{
		Timeout: ptr.Ptr[int](int(timeout / time.Second)),
	})
	if err != nil {
		return err
	}

	err = c.cli.ContainerRemove(ctx, c.containerID, dockerContainer.RemoveOptions{
		RemoveVolumes: true,
		Force:         true,
	})
	if err != nil {
		return err
	}

	return nil
}

func (c *container) pullImage(ctx context.Context) error {
	isLatest := strings.HasSuffix(c.image, ":latest")

	err := c.isImagePulled(ctx)
	if err != nil && err != errImageIsNotPulled {
		return err
	}

	if isLatest || err == errImageIsNotPulled {
		rc, err := c.cli.ImagePull(ctx, c.image, image.PullOptions{})
		if err != nil {
			return errors.Wrap(err, "error pulling image")
		}
		defer func() { _ = rc.Close() }()

		// Drain the pull response to wait for the pull to complete.
		_, err = io.Copy(io.Discard, rc)
		if err != nil {
			return errors.Wrap(err, "error waiting for image pull to complete")
		}
	}

	return nil
}

func (c *container) isImagePulled(ctx context.Context) error {
	images, err := c.cli.ImageList(ctx, image.ListOptions{})
	if err != nil {
		return err
	}

	for _, image := range images {
		for _, tag := range image.RepoTags {
			if tag == c.image {
				return nil
			}
		}
	}

	return errImageIsNotPulled
}

// URL returns host & port pair to allow external connections
func (c *container) URL(proto Protocol, port uint16) (*HostPort, error) {
	log.WithFields(log.Fields{
		"proto":   proto.String(),
		"port":    port,
		"mapping": c.ports.portBindings,
	}).Trace("looking up for port ...")

	k := strconv.FormatUint(uint64(port), 10) + "/" + proto.String()
	if v, ok := c.ports.portAliases[k]; ok {
		k = v
	}

	pbs, ok := c.ports.portBindings[k]
	if !ok {
		return nil, errors.Errorf("port `%s` is not registered", k)
	}

	if len(pbs) != 1 {
		return nil, errors.New("unexpected amount of ports returned by name: mostly possible programmer error!")
	}

	dockerIP, err := DockerIP()
	if err != nil {
		return nil, err
	}

	hp := pbs[0].HostPort
	if hp == "" {
		return nil, errors.Errorf("external port is not defined for `%d`", port)
	}

	p, err := strconv.ParseUint(hp, 10, 16)
	if err != nil {
		return nil, err
	}

	return &HostPort{
		Host: dockerIP,
		Port: uint16(p),
	}, nil
}
