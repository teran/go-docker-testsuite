// Package frr_test contains the OSPF enablement test for the FRR wrapper: a
// config that runs ospfd (via WithDaemons) and verifies the daemon comes up and
// answers `show ip ospf` with the configured router-id. It runs against the
// default image (images.FRR).
package frr_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/teran/go-docker-testsuite/applications/frr"
)

// TestOSPFRunning starts FRR with ospfd enabled and asserts the daemon is
// actually running (not just configured) and that OSPF answers with the
// configured router-id. This exercises the WithDaemons option on the OSPF path.
func TestOSPFRunning(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()

	ospfCfg := []byte(`hostname frr
frr defaults traditional
!
router ospf
 router-id 10.0.0.1
!
`)

	app, err := frr.NewWithConfigT(t, ctx, ospfCfg, frr.WithDaemons("bgpd", "ospfd"))
	require.NoError(t, err)
	require.NotNil(t, app)

	// ospfd must be among the running daemons.
	out, err := app.ExecVTY(ctx, "show daemons")
	require.NoError(t, err)
	require.Contains(t, string(out), "ospfd")

	// `show ip ospf` must succeed (exit 0) and report the configured router-id.
	out, err = app.ExecVTY(ctx, "show ip ospf")
	require.NoError(t, err)
	require.Contains(t, string(out), "10.0.0.1")
}
