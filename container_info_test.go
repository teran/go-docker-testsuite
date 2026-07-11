package docker

import (
	"strconv"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func init() {
	log.SetLevel(log.TraceLevel)
}

func TestGetExternalPortMapping(t *testing.T) {
	r := require.New(t)

	t.Setenv("DOCKER_HOST", "tcp://1.1.1.1:9874")

	var count uint16 = 11000
	pb := NewPortBindingsWithPortAllocator(func(proto Protocol, port uint16) (string, uint16, []string, error) {
		count++
		return strconv.FormatUint(uint64(port), 10) + "/" + proto.String(), count, []string{}, nil
	}).
		PortDNAT(ProtoTCP, 1234).
		PortDNAT(ProtoUDP, 4567)

	ci := &containerInfo{
		ports:        pb,
		dockerHostIP: "1.1.1.1",
	}

	// Valid port lookup
	p, err := ci.GetExternalPortMapping(ProtoTCP, 1234)
	r.NoError(err)
	r.Equal(uint16(11001), p)

	// UDP port lookup
	p, err = ci.GetExternalPortMapping(ProtoUDP, 4567)
	r.NoError(err)
	r.Equal(uint16(11002), p)
}

func TestGetExternalPortMappingWithAliases(t *testing.T) {
	r := require.New(t)

	t.Setenv("DOCKER_HOST", "tcp://1.1.1.1:9874")

	var count uint16 = 12000
	pb := NewPortBindingsWithPortAllocator(func(proto Protocol, port uint16) (string, uint16, []string, error) {
		count++
		return strconv.FormatUint(uint64(port), 10) + "/" + proto.String(), count, []string{
			strconv.FormatUint(uint64(count+1000), 10) + "/" + proto.String(),
		}, nil
	}).
		PortDNAT(ProtoTCP, 1234)

	ci := &containerInfo{
		ports:        pb,
		dockerHostIP: "1.1.1.1",
	}

	// Lookup by alias
	p, err := ci.GetExternalPortMapping(ProtoTCP, 13001)
	r.NoError(err)
	r.Equal(uint16(12001), p)
}

func TestGetExternalPortMappingNotFound(t *testing.T) {
	r := require.New(t)

	ci := &containerInfo{
		ports:        NewPortBindings(),
		dockerHostIP: "127.0.0.1",
	}

	_, err := ci.GetExternalPortMapping(ProtoTCP, 9999)
	r.Error(err)
	r.Contains(err.Error(), "port `9999/tcp` is not registered")
}

func TestGetDockerHostIP(t *testing.T) {
	r := require.New(t)

	// Resolved address
	ci := &containerInfo{dockerHostIP: "192.168.1.1"}
	ip, err := ci.GetDockerHostIP()
	r.NoError(err)
	r.Equal("192.168.1.1", ip)

	// Unresolved address
	ci2 := &containerInfo{dockerHostIP: ""}
	_, err = ci2.GetDockerHostIP()
	r.Error(err)
	r.ErrorIs(err, ErrDockerHostIPIsNotResolved)
}
