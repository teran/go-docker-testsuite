// Package frr_test contains the root integration test for the FRR wrapper. It
// runs against the default image (images.FRR) and exercises the full container
// interaction scenario, including on-demand route withdrawal.
package frr_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/teran/go-docker-testsuite/applications/frr"
)

// frrTestConfig originates 192.0.2.0/24 locally: a Null0 static puts the prefix
// into zebra's RIB and the BGP `network` statement advertises it, so the
// wrapper can assert reception and then a real on-demand withdrawal. The
// loopback neighbor keeps bgpd active and provides a peer whose remote-as is
// parsed as a real number.
//
// Keep in sync with versions/versions.go (frrTestConfig / frrWithdrawCommand).
const frrTestConfig = `hostname frr
frr defaults traditional
!
ip route 192.0.2.0/24 Null0
!
router bgp 65000
 bgp router-id 10.10.0.1
 network 192.0.2.0/24
 neighbor 10.10.0.1 remote-as 65000
 neighbor 10.10.0.1 update-source 10.10.0.1
!
`

// frrWithdrawCommand removes the `network 192.0.2.0/24` statement from the BGP
// config, which makes FRR drop the prefix from its BGP table.
//
// NOTE: merely removing the underlying static route (no ip route ... Null0) is
// NOT enough to withdraw the announcement — FRR keeps a `network` statement's
// prefix installed in the BGP table as `pathFrom: external` even after it
// leaves the RIB, so the network statement itself must be removed. This was
// verified against a live quay.io/frrouting/frr:10.7.1 container.
const frrWithdrawCommand = `configure terminal
router bgp 65000
no network 192.0.2.0/24
end
`

// TestFRR is the root integration test against the default FRR image
// (images.FRR). It covers the full interaction scenario: route reception,
// summary parsing and — the critical path for the intended anycastd use-case —
// on-demand route withdrawal.
func TestFRR(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()

	// NewWithConfigT binds the container lifecycle to the test via
	// t.Cleanup; no manual Close is required. The default image (images.FRR)
	// is used because no WithImage option is passed.
	app, err := frr.NewWithConfigT(t, ctx, []byte(frrTestConfig))
	require.NoError(t, err)
	require.NotNil(t, app)

	// Smoke: the JSON table parses (proves vtysh reaches the FRR daemons).
	routes, err := app.BGPRoutes(ctx)
	require.NoError(t, err)
	require.NotNil(t, routes)

	// Reception: the locally-originated prefix appears in the BGP table.
	require.NoError(t, app.WaitForBGPRoute(ctx, "192.0.2.0/24"))
	ok, err := app.HasBGPRoute(ctx, "192.0.2.0/24")
	require.NoError(t, err)
	require.True(t, ok)

	// Summary: the local AS and the peer's remote-as parse as real numbers.
	summary, err := app.BGPSummary(ctx)
	require.NoError(t, err)
	require.NotNil(t, summary)
	require.Equal(t, uint32(65000), summary.LocalAS)
	require.Equal(t, 1, summary.TotalPeers)
	require.Equal(t, uint32(65000), summary.Peers[0].RemoteAS)

	// Withdrawal (the critical e2e path): remove the network statement and
	// assert the prefix disappears from the BGP table.
	_, err = app.ExecVTY(ctx, frrWithdrawCommand)
	require.NoError(t, err)

	require.NoError(t, app.WaitForBGPRouteWithdrawn(ctx, "192.0.2.0/24"))
	ok, err = app.HasBGPRoute(ctx, "192.0.2.0/24")
	require.NoError(t, err)
	require.False(t, ok)

	t.Logf("frr bgp summary: router-id=%s local-as=%d peers=%d", summary.RouterID, summary.LocalAS, summary.TotalPeers)
}
