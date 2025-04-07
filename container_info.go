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
	portMapping  map[string][]Binding
	dockerHostIP string
}

func newContainerInfoFromContainer(c *container) ContainerInfo {
	addr, err := DockerIP()
	if err != nil {
		panic(err)
	}

	return &containerInfo{
		dockerHostIP: addr,
		portMapping:  c.ports.portBindings,
	}
}

func (c *containerInfo) GetExternalPortMapping(proto Protocol, port uint16) (uint16, error) {
	log.WithFields(log.Fields{
		"proto":   proto.String(),
		"port":    port,
		"mapping": c.portMapping,
	}).Trace("looking up for port ...")

	k := strconv.FormatUint(uint64(port), 10) + "/" + proto.String()
	pbs, ok := c.portMapping[k]
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
