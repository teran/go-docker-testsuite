package docker

import (
	"strconv"

	dockerContainer "github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	log "github.com/sirupsen/logrus"
)

// HostConfigSpec is just a wrapper structure to pass host configuration to the container
type HostConfigSpec struct {
	PortBindings map[string][]Binding
}

type PortAllocator func(proto Protocol, port uint16) (string, uint16, []string, error)

// Binding reflects a mapping part of the internal & external port of the container
type Binding struct {
	HostIP   string
	HostPort string
}

// NewHostConfig creates new HostConfig instance
func NewHostConfig(pb *PortBindings) (*dockerContainer.HostConfig, error) {
	pm := nat.PortMap{}
	for k, v := range pb.portBindings {
		for _, pb := range v {
			p := nat.Port(k)
			pm[p] = append(pm[p], nat.PortBinding{
				HostIP:   pb.HostIP,
				HostPort: pb.HostPort,
			})
		}
	}

	return &dockerContainer.HostConfig{
		NetworkMode:  dockerContainer.NetworkMode("default"),
		PortBindings: pm,
	}, nil
}

// PortBindings is a full mapping of internal & external docker container ports
type PortBindings struct {
	portAliases      map[string]string
	portBindings     map[string][]Binding
	tcpPortAllocator PortAllocator
}

// NewPortBindings creates new PortBindings instance
func NewPortBindings() *PortBindings {
	return NewPortBindingsWithPortAllocator(RandomPort)
}

func NewDirectPortBinding() *PortBindings {
	return NewPortBindingsWithPortAllocator(OneToOneRandomPort)
}

// NewPortBindingsWithTCPPortAllocator creates new PortBinding instance
// and allows to pass custom port allocation function
func NewPortBindingsWithPortAllocator(allocator PortAllocator) *PortBindings {
	return &PortBindings{
		portAliases:      make(map[string]string),
		portBindings:     make(map[string][]Binding),
		tcpPortAllocator: allocator,
	}
}

// PortDNAT adds new port to be exposed from the container
func (pb *PortBindings) PortDNAT(proto Protocol, port uint16) *PortBindings {
	log.WithFields(log.Fields{
		"proto": proto,
		"port":  port,
	}).Tracef("add port to port bindings")
	dockerIP, err := DockerIP()
	if err != nil {
		panic(err)
	}

	portName, externalPort, aliases, err := pb.tcpPortAllocator(proto, port)
	if err != nil {
		panic(err)
	}

	for _, alias := range aliases {
		pb.portAliases[alias] = portName
	}

	log.WithFields(log.Fields{
		"protocol": proto,
		"name":     portName,
		"source":   port,
		"exposed":  externalPort,
	}).Tracef("port mapping established")

	pb.portBindings[portName] = append(
		pb.portBindings[portName],
		Binding{
			HostIP:   dockerIP,
			HostPort: strconv.FormatUint(uint64(externalPort), 10),
		},
	)

	return pb
}

func (pb *PortBindings) portSet() nat.PortSet {
	ps := nat.PortSet{}
	for b := range pb.portBindings {
		ps[nat.Port(b)] = struct{}{}
	}

	log.WithFields(log.Fields{
		"ports": ps,
	}).Trace("port set retrieved")

	return ps
}
