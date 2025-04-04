package docker

import (
	"bufio"
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	dockerContainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	ptr "github.com/teran/go-ptr"
)

const (
	defaultStopTimeout = 1 * time.Minute
)

var errImageIsNotPulled = errors.New("image is not pulled")

type (
	ContainerID = string
	NetworkID   = string
)

// Container exposes interface to control the container runtime
type Container interface {
	AwaitOutput(ctx context.Context, m Matcher) error
	Close(ctx context.Context) error
	Name() string
	NetworkAttach(networkID string) error
	Ping(ctx context.Context) error
	Run(ctx context.Context) error
	URL(proto Protocol, port uint16) (*HostPort, error)
}

type container struct {
	cli *client.Client

	name        string
	image       string
	env         Environment
	cmd         []string
	containerID ContainerID
	networkID   NetworkID
	ports       *PortBindings
}

// New creates new container instance from remote docker image
func NewContainer(name, image string, cmd []string, environment Environment, ports *PortBindings) (Container, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}

	return NewContainerWithClient(cli, name, image, cmd, environment, ports)
}

// NewContainerWithClient creates new container from remote docker image and allows
// to pass custom docker.Client instance
func NewContainerWithClient(cli *client.Client, name, image string, cmd []string, env Environment, ports *PortBindings) (Container, error) {
	log.WithFields(log.Fields{
		"name":  name,
		"image": image,
	}).Debugf("initializing container")

	return &container{
		cli:   cli,
		name:  name,
		image: image,
		cmd:   cmd,
		env:   env,
		ports: ports,
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

func (c *container) Name() string {
	return c.name
}

// NetworkAttach attachs the container to the specific network
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
		Env:          c.env.list(),
		Cmd:          c.cmd,
		ExposedPorts: c.ports.portSet(),
	}

	networkConfig := &network.NetworkingConfig{}

	hostConfig, err := NewHostConfig(c.ports)
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

// Close cleans up the env (stops & removes the image)
func (c *container) Close(ctx context.Context) error {
	timeout := defaultStopTimeout
	if dl, ok := ctx.Deadline(); ok {
		timeout = time.Until(dl)
	}

	err := c.cli.ContainerStop(ctx, c.containerID, dockerContainer.StopOptions{
		Timeout: ptr.Int(int(timeout / time.Second)),
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

	return c.cli.Close()
}

func (c *container) pullImage(ctx context.Context) error {
	isLatest := strings.HasSuffix(c.image, ":latest")

	err := c.isImagePulled(ctx)
	if err != nil && err != errImageIsNotPulled {
		return err
	}

	if isLatest || err == errImageIsNotPulled {
		image, err := c.cli.ImagePull(ctx, c.image, image.PullOptions{})
		if err != nil {
			return errors.Wrap(err, "error pulling image")
		}

		_, err = c.cli.ImageLoad(ctx, image, false)
		if err != nil {
			return errors.Wrap(err, "error loading image")
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
	k := fmt.Sprintf("%d/%s", port, proto.String())
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
