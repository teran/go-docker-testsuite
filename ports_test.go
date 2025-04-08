package docker

import (
	"strconv"
	"testing"

	dockerContainer "github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func init() {
	log.SetLevel(log.TraceLevel)
}

func TestPortBindings(t *testing.T) {
	r := require.New(t)

	t.Setenv("DOCKER_HOST", "tcp://1.1.1.1:9874")

	var count uint16 = 12000
	pb := NewPortBindingsWithPortAllocator(func(proto Protocol, port uint16) (string, uint16, []string, error) {
		count++
		return strconv.FormatUint(uint64(port), 10) + "/" + proto.String(), count, []string{}, nil
	}).
		PortDNAT(ProtoTCP, 1234).
		PortDNAT(ProtoUDP, 4567)
	r.Equal(map[string][]Binding{
		"1234/tcp": {
			{
				HostIP:   "1.1.1.1",
				HostPort: "12001",
			},
		},
		"4567/udp": {
			{
				HostIP:   "1.1.1.1",
				HostPort: "12002",
			},
		},
	}, pb.portBindings)
}

func TestPortBindingsWithAliases(t *testing.T) {
	r := require.New(t)

	t.Setenv("DOCKER_HOST", "tcp://1.1.1.1:9874")

	var count uint16 = 12000
	pb := NewPortBindingsWithPortAllocator(func(proto Protocol, port uint16) (string, uint16, []string, error) {
		count++
		return strconv.FormatUint(uint64(port), 10) + "/" + proto.String(), count, []string{
			strconv.FormatUint(uint64(count+1000), 10) + "/" + proto.String(),
		}, nil
	}).
		PortDNAT(ProtoTCP, 1234).
		PortDNAT(ProtoUDP, 4567)
	r.Equal(map[string][]Binding{
		"1234/tcp": {
			{
				HostIP:   "1.1.1.1",
				HostPort: "12001",
			},
		},
		"4567/udp": {
			{
				HostIP:   "1.1.1.1",
				HostPort: "12002",
			},
		},
	}, pb.portBindings)
	r.Equal(map[string]string{
		"13001/tcp": "1234/tcp",
		"13002/udp": "4567/udp",
	}, pb.portAliases)
}

func TestNewHostConfig(t *testing.T) {
	r := require.New(t)

	t.Setenv("DOCKER_HOST", "tcp://1.1.1.1:9874")

	var count uint16 = 12000
	pb := NewPortBindingsWithPortAllocator(func(proto Protocol, port uint16) (string, uint16, []string, error) {
		count++
		return strconv.FormatUint(uint64(port), 10) + "/" + proto.String(), count, []string{}, nil
	}).
		PortDNAT(ProtoTCP, 1234).
		PortDNAT(ProtoUDP, 4567)

	hc, err := NewHostConfig(pb)
	r.NoError(err)
	r.Equal(&dockerContainer.HostConfig{
		NetworkMode: "default",
		PortBindings: nat.PortMap{
			"1234/tcp": []nat.PortBinding{
				{
					HostIP:   "1.1.1.1",
					HostPort: "12001",
				},
			},
			"4567/udp": []nat.PortBinding{
				{
					HostIP:   "1.1.1.1",
					HostPort: "12002",
				},
			},
		},
	}, hc)
	r.Equal(nat.PortSet{
		"1234/tcp": struct{}{},
		"4567/udp": struct{}{},
	}, pb.portSet())
}
