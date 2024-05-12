package docker

import "fmt"

// Protocol to be NAT'ed from the container
type Protocol string

const (
	// ProtoTCP ...
	ProtoTCP Protocol = "tcp"

	// ProtoUDP ...
	ProtoUDP Protocol = "udp"
)

// String returns the string representation of the protocol
func (p Protocol) String() string {
	return string(p)
}

// HostPort allows to return host & port from the URL method
type HostPort struct {
	Host string
	Port uint16
}

// String returns IP:Port pair
func (hp *HostPort) String() string {
	return fmt.Sprintf("%s:%d", hp.Host, hp.Port)
}
