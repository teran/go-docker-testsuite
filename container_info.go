package docker

import (
	"strconv"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

var (
	ErrPortNotMapped             = errors.New("port not mapped")
	ErrDockerHostIPIsNotResolved = errors.New("docker host IP address cannot be resolved")
)

type ContainerInfo interface {
	GetExternalPortMapping(Protocol, uint16) (uint16, error)
	GetDockerHostIP() (string, error)
}

type containerInfo struct {
	ports        *PortBindings
	dockerHostIP string
}

func newContainerInfoFromContainer(c *container) ContainerInfo {
	addr, err := DockerIP()
	if err != nil {
		panic(err)
	}

	return &containerInfo{
		dockerHostIP: addr,
		ports:        c.ports,
	}
}

func (c *containerInfo) GetExternalPortMapping(proto Protocol, port uint16) (uint16, error) {
	log.WithFields(log.Fields{
		"proto":   proto.String(),
		"port":    port,
		"mapping": c.ports,
	}).Trace("looking up for port ...")

	k := strconv.FormatUint(uint64(port), 10) + "/" + proto.String()

	if v, ok := c.ports.portAliases[k]; ok {
		k = v
	}

	pbs, ok := c.ports.portBindings[k]
	if !ok {
		return 0, errors.Errorf("port `%s` is not registered", k)
	}

	p, err := strconv.ParseUint(pbs[0].HostPort, 10, 16)
	if err != nil {
		return 0, errors.Wrap(err, "error parsing port number")
	}
	return uint16(p), nil
}

func (c *containerInfo) GetDockerHostIP() (string, error) {
	if c.dockerHostIP == "" {
		return "", ErrDockerHostIPIsNotResolved
	}
	return c.dockerHostIP, nil
}
