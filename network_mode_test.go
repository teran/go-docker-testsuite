package docker

import (
	"testing"

	dockerContainer "github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/require"
)

// TestWithNetworkMode verifies that the NetworkMode options set
// HostConfig.NetworkMode to the expected value when applied to a HostConfig
// built via NewHostConfig.
func TestWithNetworkMode(t *testing.T) {
	r := require.New(t)

	testCases := []struct {
		name string
		opt  ContainerOption
		want dockerContainer.NetworkMode
	}{
		{
			name: "bridge",
			opt:  WithNetworkMode(NetworkModeBridge),
			want: dockerContainer.NetworkMode("bridge"),
		},
		{
			name: "host",
			opt:  WithNetworkMode(NetworkModeHost),
			want: dockerContainer.NetworkMode("host"),
		},
		{
			name: "none",
			opt:  WithNetworkMode(NetworkModeNone),
			want: dockerContainer.NetworkMode("none"),
		},
		{
			name: "host-shortcut",
			opt:  WithHostNetwork(),
			want: dockerContainer.NetworkMode("host"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			hc, err := NewHostConfig(NewPortBindings(), tc.opt)
			r.NoError(err)
			r.Equal(tc.want, hc.NetworkMode)
		})
	}
}

// TestNewHostConfigNetworkModeDefault verifies the default network mode of a
// freshly built HostConfig is "default" (Docker's bridge-network alias).
func TestNewHostConfigNetworkModeDefault(t *testing.T) {
	r := require.New(t)

	hc, err := NewHostConfig(NewPortBindings())
	r.NoError(err)
	r.Equal(dockerContainer.NetworkMode("default"), hc.NetworkMode)
}

// TestNewHostConfigWithNetworkModeOverridesDefault verifies that applying
// WithNetworkMode overrides the implicit "default" value.
func TestNewHostConfigWithNetworkModeOverridesDefault(t *testing.T) {
	r := require.New(t)

	hc, err := NewHostConfig(NewPortBindings(), WithNetworkMode(NetworkModeHost))
	r.NoError(err)
	r.Equal(dockerContainer.NetworkMode("host"), hc.NetworkMode)
}

// TestNewHostConfigNetworkModeOptionOrdering verifies that when multiple
// network-mode options are applied, the last one wins (option ordering).
func TestNewHostConfigNetworkModeOptionOrdering(t *testing.T) {
	r := require.New(t)

	hc, err := NewHostConfig(
		NewPortBindings(),
		WithNetworkMode(NetworkModeBridge),
		WithNetworkMode(NetworkModeHost),
	)
	r.NoError(err)
	r.Equal(dockerContainer.NetworkMode("host"), hc.NetworkMode)
}
