// Package versions provides a versioned integration test suite for the FRR
// wrapper. Each concrete version pins an image and runs the suite, so every
// supported FRR image is exercised against the same scenario.
package versions

import (
	"context"

	"github.com/stretchr/testify/suite"

	"github.com/teran/go-docker-testsuite/applications/frr"
)

// frrTestConfig originates 192.0.2.0/24 locally: a Null0 static puts the prefix
// into zebra's RIB and the BGP `network` statement advertises it, so the
// wrapper can assert reception and then a real on-demand withdrawal. The
// loopback neighbor keeps bgpd active and provides a peer whose remote-as is
// parsed as a real number.
//
// Keep in sync with the root test applications/frr/frr_test.go (frrTestConfig /
// frrWithdrawCommand).
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

type testSuite struct {
	suite.Suite

	ctx   context.Context
	image string
}

// New returns a versioned FRR suite bound to the given context and image.
func New(ctx context.Context, image string) *testSuite {
	return &testSuite{
		ctx:   ctx,
		image: image,
	}
}

// TestBGPInteraction is a self-contained scenario that does not need a host BGP
// speaker: FRR originates a prefix locally (a Null0 static puts 192.0.2.0/24
// into zebra's RIB, and the BGP `network` statement advertises it), then we
// assert the wrapper receives it, parses the summary, and — the critical path
// for the intended anycastd use-case — withdraws the route on demand. This
// exercises the core behavior against a live FRR daemon, mirroring the root
// test in applications/frr/frr_test.go.
func (s *testSuite) TestBGPInteraction() {
	r := s.Require()

	app, err := frr.NewWithConfig(s.ctx, []byte(frrTestConfig), frr.WithImage(s.image))
	r.NoError(err)
	r.NotNil(app)

	defer func() {
		err := app.Close(s.ctx)
		r.NoError(err)
	}()

	// Smoke: the JSON table parses (proves vtysh reaches the FRR daemons).
	routes, err := app.BGPRoutes(s.ctx)
	r.NoError(err)
	r.NotNil(routes)

	// Reception: the locally-originated prefix appears in the BGP table.
	r.NoError(app.WaitForBGPRoute(s.ctx, "192.0.2.0/24"))
	ok, err := app.HasBGPRoute(s.ctx, "192.0.2.0/24")
	r.NoError(err)
	r.True(ok)

	// Summary: the local AS and the peer's remote-as parse as real numbers.
	summary, err := app.BGPSummary(s.ctx)
	r.NoError(err)
	r.NotNil(summary)
	r.Equal(uint32(65000), summary.LocalAS)
	r.Equal(1, summary.TotalPeers)
	r.Equal(uint32(65000), summary.Peers[0].RemoteAS)

	// Withdrawal (the critical e2e path): remove the network statement and
	// assert the prefix disappears from the BGP table.
	_, err = app.ExecVTY(s.ctx, frrWithdrawCommand)
	r.NoError(err)

	r.NoError(app.WaitForBGPRouteWithdrawn(s.ctx, "192.0.2.0/24"))
	ok, err = app.HasBGPRoute(s.ctx, "192.0.2.0/24")
	r.NoError(err)
	r.False(ok)

	s.T().Logf("frr bgp summary: router-id=%s local-as=%d peers=%d", summary.RouterID, summary.LocalAS, summary.TotalPeers)
}
