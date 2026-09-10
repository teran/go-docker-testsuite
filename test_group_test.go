package docker

import (
	"context"
	"testing"
	"time"

	"github.com/docker/docker/api/types/network"
	"github.com/stretchr/testify/require"
)

func TestTestGroupBindToT(t *testing.T) {
	requireDocker(t)

	var (
		networkID NetworkID
		cid1      ContainerID
		cid2      ContainerID
	)

	t.Run("group", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
		defer cancel()

		c1, err := NewContainer("testgroup-server", busyboxImage,
			[]string{"sleep", "300"}, NewEnvironment(), NewPortBindings())
		require.NoError(t, err)

		c2, err := NewContainer("testgroup-client", busyboxImage,
			[]string{"sleep", "300"}, NewEnvironment(), NewPortBindings())
		require.NoError(t, err)

		g, err := NewGroupT(t, "test-group",
			NewApplication(c1),
			NewApplication(c2),
		)
		require.NoError(t, err)
		require.NoError(t, g.Run(ctx))

		cid1 = c1.ID()
		cid2 = c2.ID()
		networkID = g.g.(*group).networkID
		require.NotEmpty(t, cid1)
		require.NotEmpty(t, cid2)
		require.NotEmpty(t, networkID)

		// The members and the network must be present while running.
		cli := newDockerClient(t)
		_, err = cli.ContainerInspect(ctx, cid1)
		require.NoError(t, err)
		_, err = cli.ContainerInspect(ctx, cid2)
		require.NoError(t, err)
		_, err = cli.NetworkInspect(ctx, networkID, network.InspectOptions{})
		require.NoError(t, err)
	})

	// After the subtest, the group's t.Cleanup removed members and the network,
	// in the correct order (containers before network).
	requireContainerGone(t, cid1)
	requireContainerGone(t, cid2)
	requireNetworkGone(t, networkID)
}
