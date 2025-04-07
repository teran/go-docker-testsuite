package docker

import (
	"os"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func init() {
	log.SetLevel(log.TraceLevel)
}

func TestRandomPortTCP(t *testing.T) {
	r := require.New(t)

	name, port, aliases, err := RandomPort(ProtoTCP, 12345)
	r.NoError(err)
	r.Equal("12345/tcp", name)
	r.NotZero(port)
	r.Equal([]string{}, aliases)
}

func TestOneToOneRandomPort(t *testing.T) {
	r := require.New(t)

	name, port, aliases, err := OneToOneRandomPort(ProtoTCP, 12345)
	r.NoError(err)
	r.True(strings.HasSuffix(name, name))
	r.NotZero(port)
	r.Equal([]string{
		"12345/tcp",
	}, aliases)
}

func TestDockerIP(t *testing.T) {
	r := require.New(t)

	// Empty DOCKER_HOST
	_ = os.Unsetenv("DOCKER_HOST")
	ip, err := DockerIP()
	r.NoError(err)
	r.Equal("127.0.0.1", ip)

	// Valid DOCKER_HOST value for remote instance
	_ = os.Setenv("DOCKER_HOST", "tcp://1.1.1.1:2376")
	ip, err = DockerIP()
	r.NoError(err)
	r.Equal("1.1.1.1", ip)

	// Invalid DOCKER_HOST value
	_ = os.Setenv("DOCKER_HOST", "1.1.1.1")
	_, err = DockerIP()
	r.Error(err)
	r.Equal("malformed DOCKER_HOST value: empty host or port value", err.Error())
}
