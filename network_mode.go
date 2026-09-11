package docker

import dockerContainer "github.com/docker/docker/api/types/container"

// NetworkMode enumerates Docker container network modes (roadmap #19).
type NetworkMode string

const (
	// NetworkModeBridge is the default: the container sits on the Docker
	// bridge with explicit port mappings (HostConfig.NetworkMode "default").
	NetworkModeBridge NetworkMode = "bridge"

	// NetworkModeHost shares the host network namespace: 127.0.0.1 inside the
	// container IS the host's loopback, and no port mappings apply. Under this
	// mode Docker ignores port bindings.
	NetworkModeHost NetworkMode = "host"

	// NetworkModeNone disables networking entirely. Under this mode Docker
	// ignores port bindings.
	NetworkModeNone NetworkMode = "none"
)

// WithNetworkMode sets the container's network mode
// (HostConfig.NetworkMode). Note that under NetworkModeHost (and
// NetworkModeNone) Docker ignores port bindings, so any configured
// PortDNAT mappings are effectively dropped.
func WithNetworkMode(mode NetworkMode) ContainerOption {
	return func(hc *dockerContainer.HostConfig) {
		hc.NetworkMode = dockerContainer.NetworkMode(mode)
	}
}

// WithHostNetwork runs the container on the host network namespace
// (HostConfig.NetworkMode "host"): 127.0.0.1 inside the container is the
// host's loopback, and Docker ignores port bindings.
func WithHostNetwork() ContainerOption {
	return WithNetworkMode(NetworkModeHost)
}
