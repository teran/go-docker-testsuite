package docker

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// DockerIP returns docker node IP address for further connectivity usage
func DockerIP() (string, error) {
	dockerIP := "127.0.0.1"

	dockerHost := os.Getenv("DOCKER_HOST")
	log.WithFields(log.Fields{
		"docker_host": dockerHost,
	}).Trace("DOCKER_HOST value discovered")

	if dockerHost != "" {
		u, err := url.Parse(dockerHost)
		if err != nil {
			return "", errors.Wrap(err, "error parsing DOCKER_HOST value")
		}

		parts := strings.Split(u.Host, ":")
		if len(parts) < 1 || parts[0] == "" {
			return "", errors.New("malformed DOCKER_HOST value: empty host or port value")
		}

		dockerIP = parts[0]
	}

	return dockerIP, nil
}

// RandomPortTCP makes a query to the kernel about free high range IP address
// NOTE: it have some probability impact so could fail in the really small amount
// of cases
func RandomPortTCP() (uint16, error) {
	dockerIP, err := DockerIP()
	if err != nil {
		return 0, err
	}

	bindIP := "127.0.0.1"
	if dockerIP != "127.0.0.1" {
		bindIP = "0.0.0.0"
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("%s:0", bindIP))
	if err != nil {
		return 0, err
	}

	defer ln.Close()

	_, p, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		return 0, err
	}

	port, err := strconv.ParseUint(p, 10, 16)
	if err != nil {
		return 0, err
	}

	log.WithFields(log.Fields{
		"port": port,
	}).Tracef("random TCP port allocated")

	return uint16(port), nil
}
