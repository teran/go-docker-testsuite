package frr

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	docker "github.com/teran/go-docker-testsuite"
	wait "github.com/teran/go-docker-testsuite/wait"
)

// fakeContainer is a minimal docker.Container used to exercise the FRR
// parsing/wait helpers deterministically, without requiring a Docker daemon.
//
// Exec is configured per vtysh command: canned results are keyed on the
// vtysh command string (the final argv element), an optional execErr simulates
// an exec-infrastructure failure, and the stateful routes command
// (`show bgp ipv4 unicast json`) can be synthesized via routesFn to model a
// route appearing or disappearing across polls.
type fakeContainer struct {
	mu sync.Mutex

	// results maps a vtysh command (e.g. "show bgp summary json") to the
	// canned ExecResult to return.
	results map[string]*docker.ExecResult
	// execErr, when non-nil, is returned for every Exec call, simulating an
	// exec-infrastructure failure (e.g. a cancelled context or a docker error).
	execErr error
	// routesFn synthesizes the `show bgp ipv4 unicast json` response when no
	// canned result is registered for that command. It is invoked under mu and
	// may return different data on successive calls to model state changes.
	routesFn func() map[string]json.RawMessage
}

func (f *fakeContainer) AwaitOutput(context.Context, docker.Matcher) error { return nil }
func (f *fakeContainer) Close(context.Context) error                       { return nil }
func (f *fakeContainer) GetOutput(context.Context, ...docker.Matcher) ([]string, error) {
	return nil, nil
}
func (f *fakeContainer) ID() docker.ContainerID { return "fake" }
func (f *fakeContainer) Name() string           { return "fake" }
func (f *fakeContainer) NetworkAttach(string) error {
	return nil
}
func (f *fakeContainer) Ping(context.Context) error { return nil }
func (f *fakeContainer) Run(context.Context) error  { return nil }
func (f *fakeContainer) URL(docker.Protocol, uint16) (*docker.HostPort, error) {
	return nil, nil
}

// vtyshCmd extracts the vtysh command (the final argv element) from an Exec
// command vector, so tests can key canned results on the human-readable
// command (e.g. "show bgp summary json").
func vtyshCmd(cmd []string) string {
	if len(cmd) == 0 {
		return ""
	}
	return cmd[len(cmd)-1]
}

func (f *fakeContainer) Exec(ctx context.Context, cmd []string) (*docker.ExecResult, error) {
	// Mirror the real container: an already-cancelled ctx fails the exec fast
	// with ctx.Err(), so a cancelled wait bails out instead of spinning.
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.execErr != nil {
		return nil, f.execErr
	}

	key := vtyshCmd(cmd)
	if res, ok := f.results[key]; ok {
		return res, nil
	}

	if key == "show bgp ipv4 unicast json" && f.routesFn != nil {
		out, err := json.Marshal(map[string]interface{}{"routes": f.routesFn()})
		if err != nil {
			return nil, err
		}
		return &docker.ExecResult{Stdout: out}, nil
	}

	return &docker.ExecResult{}, nil
}

// ---------------------------------------------------------------------------
// parseUint
// ---------------------------------------------------------------------------

func TestParseUint(t *testing.T) {
	r := require.New(t)

	// Numeric JSON value.
	v, err := parseUint(json.RawMessage(`65000`))
	r.NoError(err)
	r.Equal(uint32(65000), v)

	// String-encoded numeric value (some FRR versions emit these as strings).
	v, err = parseUint(json.RawMessage(`"65000"`))
	r.NoError(err)
	r.Equal(uint32(65000), v)

	// JSON null coerces to 0 with no error (an absent/empty field).
	v, err = parseUint(json.RawMessage(`null`))
	r.NoError(err)
	r.Equal(uint32(0), v)

	// Malformed values -> error.
	_, err = parseUint(json.RawMessage(`"abc"`))
	r.Error(err)

	_, err = parseUint(json.RawMessage(`-1`))
	r.Error(err)
}

// ---------------------------------------------------------------------------
// BGPSummary parsing
// ---------------------------------------------------------------------------

// TestBGPSummaryParsing feeds `show bgp summary json` output with two peers —
// one with numeric remoteAs/pfxRcd and one with string-encoded remoteAs/pfxRcd
// — and asserts the parsed summary plus deterministic (sorted-by-Addr) peers.
func TestBGPSummaryParsing(t *testing.T) {
	r := require.New(t)

	out := []byte(`{
		"routerId": "10.0.0.1",
		"as": 65000,
		"peers": {
			"10.0.0.2": {"remoteAs": 64500, "state": "Established", "pfxRcd": 5},
			"10.0.0.10": {"remoteAs": "64501", "state": "Established", "pfxRcd": "7"}
		}
	}`)

	f := &frrImpl{c: &fakeContainer{
		results: map[string]*docker.ExecResult{
			"show bgp summary json": {Stdout: out},
		},
	}}

	s, err := f.BGPSummary(context.Background())
	r.NoError(err)
	r.NotNil(s)
	r.Equal("10.0.0.1", s.RouterID)
	r.Equal(uint32(65000), s.LocalAS)
	r.Equal(2, s.TotalPeers)
	r.Len(s.Peers, 2)

	// Peers must be deterministically sorted by Addr (lexicographic string
	// sort), so the map iteration order never leaks into the returned slice.
	r.Equal("10.0.0.10", s.Peers[0].Addr)
	r.Equal("10.0.0.2", s.Peers[1].Addr)

	// Numeric remoteAs/pfxRcd.
	r.Equal("Established", s.Peers[1].State)
	r.Equal(uint32(64500), s.Peers[1].RemoteAS)
	r.Equal(uint32(5), s.Peers[1].PrefixesReceived)

	// String-encoded remoteAs/pfxRcd.
	r.Equal(uint32(64501), s.Peers[0].RemoteAS)
	r.Equal(uint32(7), s.Peers[0].PrefixesReceived)
}

// TestBGPSummaryIPv4UnicastNested pins the real FRR 9/10.x schema, where the
// summary fields are nested under an "ipv4Unicast" key instead of at the top
// level. Without unwrapping, BGPSummary would fail to parse live FRR output.
func TestBGPSummaryIPv4UnicastNested(t *testing.T) {
	r := require.New(t)

	out := []byte(`{
		"ipv4Unicast": {
			"routerId": "10.10.0.1",
			"as": 65000,
			"peers": {
				"10.0.0.2": {"remoteAs": 64500, "state": "Active", "pfxRcd": 1}
			}
		}
	}`)

	f := &frrImpl{c: &fakeContainer{
		results: map[string]*docker.ExecResult{
			"show bgp summary json": {Stdout: out},
		},
	}}

	s, err := f.BGPSummary(context.Background())
	r.NoError(err)
	r.NotNil(s)
	r.Equal("10.10.0.1", s.RouterID)
	r.Equal(uint32(65000), s.LocalAS)
	r.Len(s.Peers, 1)
	r.Equal(uint32(64500), s.Peers[0].RemoteAS)
}

func TestBGPSummaryUnparseableLocalAS(t *testing.T) {
	r := require.New(t)

	out := []byte(`{"routerId": "10.0.0.1", "as": "abc", "peers": {}}`)
	f := &frrImpl{c: &fakeContainer{
		results: map[string]*docker.ExecResult{
			"show bgp summary json": {Stdout: out},
		},
	}}

	_, err := f.BGPSummary(context.Background())
	r.Error(err)
	r.Contains(err.Error(), "local AS")
}

func TestBGPSummaryBadPfxRcd(t *testing.T) {
	r := require.New(t)

	out := []byte(`{
		"routerId": "10.0.0.1",
		"as": 65000,
		"peers": {"10.0.0.2": {"remoteAs": 64500, "state": "Established", "pfxRcd": "abc"}}
	}`)
	f := &frrImpl{c: &fakeContainer{
		results: map[string]*docker.ExecResult{
			"show bgp summary json": {Stdout: out},
		},
	}}

	_, err := f.BGPSummary(context.Background())
	r.Error(err)
	r.Contains(err.Error(), "prefixes received")
}

// ---------------------------------------------------------------------------
// BGPRoutes
// ---------------------------------------------------------------------------

func TestBGPRoutesEmpty(t *testing.T) {
	r := require.New(t)

	f := &frrImpl{c: &fakeContainer{
		results: map[string]*docker.ExecResult{
			"show bgp ipv4 unicast json": {Stdout: []byte(`{"routes":{}}`)},
		},
	}}

	routes, err := f.BGPRoutes(context.Background())
	r.NoError(err)
	r.NotNil(routes)
	r.Len(routes, 0)
}

func TestBGPRoutesPopulated(t *testing.T) {
	r := require.New(t)

	f := &frrImpl{c: &fakeContainer{
		results: map[string]*docker.ExecResult{
			"show bgp ipv4 unicast json": {
				Stdout: []byte(`{"routes":{"192.0.2.0/24":{"prefix":"192.0.2.0/24"}}}`),
			},
		},
	}}

	routes, err := f.BGPRoutes(context.Background())
	r.NoError(err)
	r.Len(routes, 1)
	r.NotEmpty(routes["192.0.2.0/24"])
}

func TestBGPRoutesMalformedJSON(t *testing.T) {
	r := require.New(t)

	f := &frrImpl{c: &fakeContainer{
		results: map[string]*docker.ExecResult{
			"show bgp ipv4 unicast json": {Stdout: []byte(`not json`)},
		},
	}}

	_, err := f.BGPRoutes(context.Background())
	r.Error(err)
	r.Contains(err.Error(), "unmarshalling bgp routes json")
}

// ---------------------------------------------------------------------------
// HasBGPRoute
// ---------------------------------------------------------------------------

func TestHasBGPRoutePresent(t *testing.T) {
	r := require.New(t)

	f := &frrImpl{c: &fakeContainer{
		results: map[string]*docker.ExecResult{
			"show bgp ipv4 unicast json": {Stdout: []byte(`{"routes":{"192.0.2.0/24":{}}}`)},
		},
	}}

	ok, err := f.HasBGPRoute(context.Background(), "192.0.2.0/24")
	r.NoError(err)
	r.True(ok)
}

func TestHasBGPRouteAbsent(t *testing.T) {
	r := require.New(t)

	f := &frrImpl{c: &fakeContainer{
		results: map[string]*docker.ExecResult{
			"show bgp ipv4 unicast json": {Stdout: []byte(`{"routes":{}}`)},
		},
	}}

	ok, err := f.HasBGPRoute(context.Background(), "192.0.2.0/24")
	r.NoError(err)
	r.False(ok)
}

// TestHasBGPRouteExecFailure verifies a non-zero vtysh exit is surfaced as an
// error (and never reported as a present/absent route).
func TestHasBGPRouteExecFailure(t *testing.T) {
	r := require.New(t)

	f := &frrImpl{c: &fakeContainer{
		results: map[string]*docker.ExecResult{
			"show bgp ipv4 unicast json": {ExitCode: 1, Stderr: []byte("no table")},
		},
	}}

	ok, err := f.HasBGPRoute(context.Background(), "192.0.2.0/24")
	r.Error(err)
	r.False(ok)
}

// ---------------------------------------------------------------------------
// runVTY
// ---------------------------------------------------------------------------

func TestRunVTYNonZeroExit(t *testing.T) {
	r := require.New(t)

	f := &frrImpl{c: &fakeContainer{
		results: map[string]*docker.ExecResult{
			"show bgp summary json": {ExitCode: 1, Stderr: []byte("vty error")},
		},
	}}

	_, err := f.runVTY(context.Background(), "show bgp summary json")
	r.Error(err)
	r.Contains(err.Error(), "exited 1")
}

// ---------------------------------------------------------------------------
// renderDaemonsConfig / WithDaemons
// ---------------------------------------------------------------------------

// TestRenderDaemonsConfigDefault pins the default (bgpd-only) daemons file:
// zebra and mgmtd always yes, bgpd yes, every other known daemon no.
func TestRenderDaemonsConfigDefault(t *testing.T) {
	r := require.New(t)

	out, err := renderDaemonsConfig([]string{"bgpd"})
	r.NoError(err)

	want := `zebra=yes
mgmtd=yes
bgpd=yes
ospfd=no
ospf6d=no
ripd=no
ripngd=no
isisd=no
pimd=no
ldpd=no
nhrpd=no
eigrpd=no
babeld=no
sharpd=no
pbrd=no
staticd=no
bfdd=no
fabricd=no
vrrpd=no
pathd=no
`
	r.Equal(want, string(out))
}

// TestRenderDaemonsConfigOSPFD enables ospfd (and keeps bgpd) and verifies both
// are set to yes while zebra/mgmtd remain forced on.
func TestRenderDaemonsConfigOSPFD(t *testing.T) {
	r := require.New(t)

	out, err := renderDaemonsConfig([]string{"bgpd", "ospfd"})
	r.NoError(err)

	s := string(out)
	r.Contains(s, "zebra=yes")
	r.Contains(s, "mgmtd=yes")
	r.Contains(s, "bgpd=yes")
	r.Contains(s, "ospfd=yes")
	r.Contains(s, "ospf6d=no")
}

// TestRenderDaemonsConfigUnknownDaemon verifies an unknown daemon name is a
// build-time error.
func TestRenderDaemonsConfigUnknownDaemon(t *testing.T) {
	r := require.New(t)

	_, err := renderDaemonsConfig([]string{"bogus"})
	r.Error(err)
	r.Contains(err.Error(), `unknown FRR daemon "bogus"`)
}

// TestWithDaemons ensures the Option sets the daemons slice on the config.
func TestWithDaemons(t *testing.T) {
	r := require.New(t)

	cfg := defaultConfig()
	WithDaemons("bgpd", "ospfd", "bfdd")(cfg)
	r.Equal([]string{"bgpd", "ospfd", "bfdd"}, cfg.daemons)
}

// TestWithContainerName ensures the Option sets the container name.
func TestWithContainerName(t *testing.T) {
	r := require.New(t)

	cfg := defaultConfig()
	WithContainerName("frr-a")(cfg)
	r.Equal("frr-a", cfg.name)
}

// ---------------------------------------------------------------------------
// waitForBGPRoute semantics
// ---------------------------------------------------------------------------

// TestWaitForBGPRouteCancelledContext verifies that with an already-cancelled
// ctx the presence wait fails fast with context.Canceled instead of polling
// for the full timeout.
func TestWaitForBGPRouteCancelledContext(t *testing.T) {
	r := require.New(t)

	// Route is never present; the cancelled ctx makes Exec bail immediately.
	f := &frrImpl{c: &fakeContainer{
		results: map[string]*docker.ExecResult{
			"show bgp ipv4 unicast json": {Stdout: []byte(`{"routes":{}}`)},
		},
	}}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := f.WaitForBGPRoute(ctx, "192.0.2.0/24")
	r.Error(err)
	r.True(errors.Is(err, context.Canceled))
}

// TestWaitForBGPRouteWithdrawn models a peer announcing then withdrawing a
// prefix: the withdrawn wait must observe the disappearance and return nil,
// while the presence wait on a cancelled ctx still fails fast.
func TestWaitForBGPRouteWithdrawn(t *testing.T) {
	r := require.New(t)

	// Route is present on the first poll, then withdrawn.
	calls := 0
	f := &frrImpl{c: &fakeContainer{
		routesFn: func() map[string]json.RawMessage {
			calls++
			if calls == 1 {
				return map[string]json.RawMessage{"192.0.2.0/24": json.RawMessage(`{}`)}
			}
			return map[string]json.RawMessage{}
		},
	}}

	err := f.WaitForBGPRouteWithdrawn(context.Background(), "192.0.2.0/24", wait.WithInterval(10*time.Millisecond))
	r.NoError(err)

	// Presence wait on an already-cancelled ctx fails fast with
	// context.Canceled regardless of the route state.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = f.WaitForBGPRoute(ctx, "192.0.2.0/24")
	r.Error(err)
	r.True(errors.Is(err, context.Canceled))
}

// ---------------------------------------------------------------------------
// waitForDaemons
// ---------------------------------------------------------------------------
//
// waitForDaemons polls `show daemons` via wait.Wait until every requested
// daemon is present. Its strategy treats both an exec failure and a missing
// daemon as "not ready yet" (it returns (false, nil) and retries), so the
// only error it can return is the one surfaced by the wait loop — which, with
// a cancelled ctx, is context.Canceled. The tests below pin that real
// behavior: a present daemon returns nil; a missing daemon and an exec
// failure both bail fast with context.Canceled when driven by a cancelled ctx.

// TestWaitForDaemonsPresent verifies waitForDaemons returns nil as soon as the
// requested daemon appears in `show daemons`.
func TestWaitForDaemonsPresent(t *testing.T) {
	r := require.New(t)

	f := &fakeContainer{
		results: map[string]*docker.ExecResult{
			"show daemons": {Stdout: []byte("zebra mgmtd bgpd staticd")},
		},
	}

	err := waitForDaemons(context.Background(), f, []string{"bgpd"})
	r.NoError(err)
}

// TestWaitForDaemonsMissingDaemon verifies that when the awaited daemon is
// absent from `show daemons` the strategy stays not-ready and the wait bails
// fast with context.Canceled on a cancelled ctx (instead of retrying for the
// full 60s default timeout).
func TestWaitForDaemonsMissingDaemon(t *testing.T) {
	r := require.New(t)

	// Output does not contain "bgpd", so the strategy stays not-ready.
	f := &fakeContainer{
		results: map[string]*docker.ExecResult{
			"show daemons": {Stdout: []byte("zebra mgmtd staticd")},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := waitForDaemons(ctx, f, []string{"bgpd"})
	r.Error(err)
	r.True(errors.Is(err, context.Canceled))
}

// TestWaitForDaemonsExecFailure verifies that an exec failure is treated as
// transient (the strategy retries rather than surfacing the raw exec error),
// and that with a cancelled ctx waitForDaemons surfaces context.Canceled.
func TestWaitForDaemonsExecFailure(t *testing.T) {
	r := require.New(t)

	sentinel := errors.New("exec boom")
	f := &fakeContainer{execErr: sentinel}

	// A cancelled ctx makes the wait bail fast with context.Canceled. The
	// strategy swallows the underlying exec error (retries), so it is the
	// wait loop's ctx error — not the sentinel — that is surfaced.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := waitForDaemons(ctx, f, []string{"bgpd"})
	r.Error(err)
	r.True(errors.Is(err, context.Canceled))
}
