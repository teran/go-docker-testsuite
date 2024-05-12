//go:build unit

package docker

import (
	"os"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func init() {
	log.SetLevel(log.TraceLevel)
}

func TestRandomPortTCP(t *testing.T) {
	r := require.New(t)

	port, err := RandomPortTCP()
	r.NoError(err)
	r.NotZero(port)
}

func TestDockerIP(t *testing.T) {
	r := require.New(t)

	// Empty DOCKER_HOST
	os.Unsetenv("DOCKER_HOST")
	ip, err := DockerIP()
	r.NoError(err)
	r.Equal("127.0.0.1", ip)

	// Valid DOCKER_HOST value for remote instance
	os.Setenv("DOCKER_HOST", "tcp://1.1.1.1:2376")
	ip, err = DockerIP()
	r.NoError(err)
	r.Equal("1.1.1.1", ip)

	// Invalid DOCKER_HOST value
	os.Setenv("DOCKER_HOST", "1.1.1.1")
	ip, err = DockerIP()
	r.Error(err)
	r.Equal("malformed DOCKER_HOST value: empty host or port value", err.Error())
}
