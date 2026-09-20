// Package frr_test contains the Group-based e2e test for the FRR wrapper: two
// FRR BGP peers on an internal docker.Group network, peering by container-name
// alias with no host port exposure. It exercises the full path the wrapper is
// built for — independent BGP peers exchanging routes over the docker network,
// including on-demand withdrawal.
package frr_test

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	docker "github.com/teran/go-docker-testsuite"
	"github.com/teran/go-docker-testsuite/applications/frr"
)

// bgpPeerConfig returns an frr.conf that originates `network` locally: a Null0
// static puts the prefix into zebra's RIB and the BGP `network` statement
// advertises it. `no bgp ebgp-requires-policy` is required so the eBGP peer
// exchanges prefixes without a route policy (FRR 9.x+ holds such peers in
// "Policy" state with zero prefixes otherwise). No neighbor line is put in
// static config — the peers are added dynamically after the Group is running,
// resolving each other's container-name alias to an IP.
func bgpPeerConfig(hostname, routerID string, localAS uint32, network string) []byte {
	return []byte(fmt.Sprintf(`hostname %s
frr defaults traditional
!
ip route %s Null0
!
router bgp %d
 no bgp ebgp-requires-policy
 bgp router-id %s
 network %s
!
`, hostname, network, localAS, routerID, network))
}

// TestFRRGroupPeering runs two FRR BGP peers on an internal docker.Group
// network, peers them by container-name alias, and verifies bidirectional route
// exchange plus a real withdrawal.
func TestFRRGroupPeering(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()

	// Two peers with distinct ASNs, router-ids and announced networks. No
	// neighbor line in static config: added dynamically after Run.
	cfgA := bgpPeerConfig("frr-a", "10.0.0.1", 65001, "192.0.2.0/24")
	cfgB := bgpPeerConfig("frr-b", "10.0.0.2", 65002, "198.51.100.0/24")

	cA, err := frr.NewContainer(ctx, cfgA, frr.WithContainerName("frr-a"))
	require.NoError(t, err)
	cB, err := frr.NewContainer(ctx, cfgB, frr.WithContainerName("frr-b"))
	require.NoError(t, err)

	g, err := docker.NewGroup("frr-peer",
		docker.NewApplication(cA),
		docker.NewApplication(cB),
	)
	require.NoError(t, err)
	defer func() { require.NoError(t, g.Close(ctx)) }()

	require.NoError(t, g.Run(ctx))

	appA := frr.NewFromContainer(cA)
	appB := frr.NewFromContainer(cB)

	// Add neighbors dynamically now that both containers are up. FRR does not
	// auto-resolve a hostname BGP neighbor (it errors "Create the peer-group
	// or interface first"), so each peer's container-name alias is resolved to
	// its IP on the internal network (via getent) and the neighbor is added by
	// that IP — the resolution still happens by name on the docker network,
	// with no host port and no hardcoded address.
	addNeighbor(t, ctx, cA, appA, 65001, "frr-b", 65002)
	addNeighbor(t, ctx, cB, appB, 65002, "frr-a", 65001)

	// A receives B's announced network; B receives A's announced network.
	require.NoError(t, appA.WaitForBGPRoute(ctx, "198.51.100.0/24"))
	require.NoError(t, appB.WaitForBGPRoute(ctx, "192.0.2.0/24"))

	// Both peers are Established with the correct remote AS.
	sA, err := appA.BGPSummary(ctx)
	require.NoError(t, err)
	sB, err := appB.BGPSummary(ctx)
	require.NoError(t, err)
	requirePeerEstablished(t, sA, 65002)
	requirePeerEstablished(t, sB, 65001)

	// Withdrawal (the critical e2e path): A stops announcing 192.0.2.0/24 and
	// B must observe the withdrawal — proving the peering is not vacuous.
	_, err = appA.ExecVTY(ctx, "configure terminal\nrouter bgp 65001\nno network 192.0.2.0/24\nend")
	require.NoError(t, err)
	require.NoError(t, appB.WaitForBGPRouteWithdrawn(ctx, "192.0.2.0/24"))
}

// requirePeerEstablished asserts the summary has exactly one peer, Established,
// with the given remote AS.
func requirePeerEstablished(t *testing.T, s *frr.BGPSummary, remoteAS uint32) {
	t.Helper()
	require.NotNil(t, s)
	require.Len(t, s.Peers, 1)
	require.Equal(t, remoteAS, s.Peers[0].RemoteAS)
	require.Equal(t, "Established", s.Peers[0].State)
}

// addNeighbor configures a BGP neighbor on app. It first resolves peerName (a
// container-name alias on the docker.Group internal network) to its IPv4
// address from inside local via getent, then adds the neighbor by that IP,
// retrying until bgpd accepts it (bgpd may still be starting right after the
// Group's Run). The group performs no FRR daemon-readiness wait, so both the
// resolution and the neighbor-add race the sibling's startup.
func addNeighbor(t *testing.T, ctx context.Context, local docker.Container, app frr.FRR, localAS uint32, peerName string, peerAS uint32) {
	t.Helper()

	peerIP := resolvePeerIP(ctx, local, peerName)
	require.NotEmpty(t, peerIP)

	cmd := fmt.Sprintf("configure terminal\nrouter bgp %d\nneighbor %s remote-as %d\nend", localAS, peerIP, peerAS)
	for {
		_, err := app.ExecVTY(ctx, cmd)
		if err == nil {
			return
		}
		select {
		case <-ctx.Done():
			require.NoError(t, err)
			return
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// resolvePeerIP resolves peerName (a container-name alias on the docker.Group
// internal network) to its IPv4 address from inside the local container, using
// getent. It retries until the alias resolves, because docker DNS registers the
// sibling peers on the internal network slightly after they start.
func resolvePeerIP(ctx context.Context, local docker.Container, peerName string) string {
	for {
		res, err := local.Exec(ctx, []string{"getent", "hosts", peerName})
		if err == nil && res.ExitCode == 0 {
			fields := strings.Fields(string(res.Stdout))
			if len(fields) > 0 {
				// Only an IPv4 address is usable as a BGP neighbor; skip any
				// v6 address getent might return first on a dual-stack network.
				if ip := net.ParseIP(fields[0]); ip != nil && ip.To4() != nil {
					return fields[0]
				}
			}
		}
		select {
		case <-ctx.Done():
			return ""
		case <-time.After(200 * time.Millisecond):
		}
	}
}
