// Package frr provides a typed wrapper around an FRR (FRRouting) container for
// integration testing.
//
// FRR is an independent, C-based BGP implementation. The wrapper is intended
// for end-to-end tests of a BGP-speaking service (e.g. the anycastd project,
// which uses GoBGP): a test can announce a prefix and assert that an
// independent BGP peer (this FRR container) receives it, and that the route
// is withdrawn when the service goes down.
package frr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	docker "github.com/teran/go-docker-testsuite"
	"github.com/teran/go-docker-testsuite/images"
	wait "github.com/teran/go-docker-testsuite/wait"
)

const (
	containerName = "frr"

	// defaultWaitTimeout bounds WaitForBGPRoute / WaitForBGPRouteWithdrawn
	// when the caller's ctx carries no deadline.
	defaultWaitTimeout = 30 * time.Second
)

const defaultFRRConfig = `hostname frr
frr defaults traditional
`

const vtyshConfig = `service integrated-vtysh-config
`

const (
	daemonsDestination = "/etc/frr/daemons"
	vtyshDestination   = "/etc/frr/vtysh.conf"
	configDestination  = "/etc/frr/frr.conf"
)

// PeerState describes a single BGP peer from `show bgp summary json`.
type PeerState struct {
	// Addr is the peer's IP address (the key in the "peers" map).
	Addr string
	// RemoteAS is the peer's autonomous system number.
	RemoteAS uint32
	// State is the BGP session state (e.g. "Established").
	State string
	// PrefixesReceived is the number of prefixes received from the peer.
	PrefixesReceived uint32
}

// BGPSummary is the parsed output of `show bgp summary json`.
type BGPSummary struct {
	// RouterID is the local BGP router-id.
	RouterID string
	// LocalAS is the local autonomous system number.
	LocalAS uint32
	// Peers is the list of BGP peers.
	Peers []PeerState
	// TotalPeers is the number of peers.
	TotalPeers int
}

// FRR is the typed client interface exposed by the FRR wrapper.
type FRR interface {
	// Container returns the wrapped docker.Container, for Group membership.
	Container() docker.Container
	// ExecVTY runs a command through FRR's vtysh and returns its stdout.
	ExecVTY(ctx context.Context, cmd string) ([]byte, error)
	// BGPRoutes returns the parsed "routes" map of
	// `show bgp ipv4 unicast json` (map[prefix]raw route data).
	BGPRoutes(ctx context.Context) (map[string]json.RawMessage, error)
	// HasBGPRoute reports whether a given prefix is currently announced in
	// the IPv4 unicast BGP table.
	HasBGPRoute(ctx context.Context, prefix string) (bool, error)
	// MustHasBGPRoute is HasBGPRoute that panics on error.
	MustHasBGPRoute(ctx context.Context, prefix string) bool
	// WaitForBGPRoute blocks until the given prefix appears in the BGP table.
	WaitForBGPRoute(ctx context.Context, prefix string, opts ...wait.Option) error
	// WaitForBGPRouteWithdrawn blocks until the given prefix disappears from
	// the BGP table.
	WaitForBGPRouteWithdrawn(ctx context.Context, prefix string, opts ...wait.Option) error
	// Peers returns the BGP peers from `show bgp summary json`.
	Peers(ctx context.Context) ([]PeerState, error)
	// BGPSummary returns the parsed `show bgp summary json`.
	BGPSummary(ctx context.Context) (*BGPSummary, error)
	// Close stops and removes the container.
	Close(ctx context.Context) error
}

// Option configures the FRR wrapper.
type Option func(*frrConfig)

type frrConfig struct {
	image   string
	name    string
	daemons []string
}

// WithImage overrides the FRR image (used by versioned integration tests).
func WithImage(image string) Option {
	return func(c *frrConfig) {
		c.image = image
	}
}

// WithContainerName sets the FRR container name (and its DNS alias on a
// docker.Group internal network). Two peers in the same Group must use
// distinct names so they can peer by name. Default: "frr".
func WithContainerName(name string) Option {
	return func(c *frrConfig) {
		c.name = name
	}
}

// knownDaemons lists the FRR protocol daemons, in the order they are emitted
// into /etc/frr/daemons. zebra and mgmtd are always force-enabled by the image
// and lead the list.
var knownDaemons = []string{
	"zebra", "mgmtd", "bgpd", "ospfd", "ospf6d", "ripd", "ripngd", "isisd",
	"pimd", "ldpd", "nhrpd", "eigrpd", "babeld", "sharpd", "pbrd", "staticd",
	"bfdd", "fabricd", "vrrpd", "pathd",
}

// WithDaemons sets the FRR protocol daemons to enable. Every other known
// daemon is set to "no" in /etc/frr/daemons. zebra and mgmtd are always
// force-enabled by the image. Default: []string{"bgpd"}.
func WithDaemons(daemons ...string) Option {
	return func(c *frrConfig) {
		c.daemons = daemons
	}
}

type frrImpl struct {
	c docker.Container
}

func defaultConfig() *frrConfig {
	return &frrConfig{
		image:   images.FRR,
		name:    containerName,
		daemons: []string{"bgpd"},
	}
}

// renderDaemonsConfig renders the /etc/frr/daemons file. zebra and mgmtd are
// always enabled; every other known daemon is set to "yes" when listed in
// enabled, else "no". All known daemons are emitted in order. An unknown
// daemon name is a build-time error.
func renderDaemonsConfig(enabled []string) ([]byte, error) {
	want := make(map[string]bool, len(enabled))
	known := make(map[string]bool, len(knownDaemons))
	for _, d := range knownDaemons {
		known[d] = true
	}

	for _, d := range enabled {
		if !known[d] {
			return nil, errors.Errorf("unknown FRR daemon %q (known: %v)", d, knownDaemons)
		}
		want[d] = true
	}

	var buf bytes.Buffer
	for _, d := range knownDaemons {
		if d == "zebra" || d == "mgmtd" {
			fmt.Fprintf(&buf, "%s=yes\n", d)
			continue
		}
		if want[d] {
			fmt.Fprintf(&buf, "%s=yes\n", d)
		} else {
			fmt.Fprintf(&buf, "%s=no\n", d)
		}
	}
	return buf.Bytes(), nil
}

// New starts an FRR container with the default (no-config) configuration.
func New(ctx context.Context) (FRR, error) {
	return NewWithImage(ctx, images.FRR)
}

// NewWithImage starts an FRR container with the default configuration using
// the given image.
func NewWithImage(ctx context.Context, image string) (FRR, error) {
	return NewWithConfig(ctx, []byte(defaultFRRConfig), WithImage(image))
}

// NewWithT is New bound to a *testing.T: the container's lifecycle is tied to
// the test and cleaned up automatically via t.Cleanup.
func NewWithT(t *testing.T, ctx context.Context) (FRR, error) {
	return NewWithImageT(t, ctx, images.FRR)
}

// NewWithImageT is NewWithImage bound to a *testing.T: the container's
// lifecycle is tied to the test and cleaned up automatically via t.Cleanup.
func NewWithImageT(t *testing.T, ctx context.Context, image string) (FRR, error) {
	return NewWithConfigT(t, ctx, []byte(defaultFRRConfig), WithImage(image))
}

// NewWithConfig starts an FRR container with the given frr.conf injected
// verbatim into /etc/frr/frr.conf. The config typically declares the BGP
// instance, a router-id and the peers to announce to.
func NewWithConfig(ctx context.Context, config []byte, opts ...Option) (FRR, error) {
	return startFRR(ctx, nil, config, opts...)
}

// NewWithConfigT is NewWithConfig bound to a *testing.T: the container's
// lifecycle is tied to the test and cleaned up automatically via t.Cleanup.
func NewWithConfigT(t *testing.T, ctx context.Context, config []byte, opts ...Option) (FRR, error) {
	return startFRR(ctx, t, config, opts...)
}

// NewContainer builds an unstarted FRR container for membership in a
// docker.Group. It does not start the container; the Group's Run() does.
// Obtain the typed FRR interface afterwards with NewFromContainer.
func NewContainer(ctx context.Context, config []byte, opts ...Option) (docker.Container, error) {
	return buildFRR(ctx, nil, config, opts...)
}

// NewContainerT is NewContainer bound to a *testing.T: the container's
// lifecycle is tied to the test and cleaned up automatically via t.Cleanup.
func NewContainerT(t *testing.T, ctx context.Context, config []byte, opts ...Option) (docker.Container, error) {
	return buildFRR(ctx, t, config, opts...)
}

// NewFromContainer wraps a running FRR docker.Container as the typed FRR
// interface — e.g. one created by NewContainer and started by a docker.Group.
func NewFromContainer(c docker.Container) FRR {
	return &frrImpl{c: c}
}

// buildFRR constructs (but does not run) an FRR container. It applies the
// options to the default config, renders /etc/frr/daemons from the enabled
// daemons, injects the three config files, sets the privileged lifecycle
// options, and uses empty port bindings (no host port exposure — FRR peers
// over the docker network). The container name/alias is cfg.name.
func buildFRR(ctx context.Context, t *testing.T, config []byte, opts ...Option) (docker.Container, error) {
	cfg := defaultConfig()
	for _, o := range opts {
		o(cfg)
	}

	daemonsCfg, err := renderDaemonsConfig(cfg.daemons)
	if err != nil {
		return nil, err
	}

	files := []docker.File{
		// daemons + vtysh.conf must be root:root (Uid/Gid 0) mode 0644 so
		// vtysh can drive the daemons. frr.conf is the primary injected
		// artifact.
		docker.FileFromBytes(daemonsDestination, daemonsCfg, 0644, 0, 0),
		docker.FileFromBytes(vtyshDestination, []byte(vtyshConfig), 0644, 0, 0),
		docker.FileFromBytes(configDestination, config, 0644, 0, 0),
	}

	// FRR needs privilege to set up routes (zebra installs routes via
	// netlink). No host port is exposed: BGP peers connect to the container
	// over the docker network instead.
	lifecycleOpts := []docker.LifecycleOption{
		docker.WithFiles(files...),
		docker.WithHostConfig(docker.WithPrivileged()),
	}

	bindings := docker.NewPortBindings()

	if t != nil {
		return docker.NewContainerWithLifecycleT(t, cfg.name, cfg.image, nil, docker.NewEnvironment(), bindings, lifecycleOpts...)
	}
	return docker.NewContainerWithLifecycle(cfg.name, cfg.image, nil, docker.NewEnvironment(), bindings, lifecycleOpts...)
}

// startFRR builds, runs and waits for the FRR container to be ready.
func startFRR(ctx context.Context, t *testing.T, config []byte, opts ...Option) (FRR, error) {
	cfg := defaultConfig()
	for _, o := range opts {
		o(cfg)
	}

	c, err := buildFRR(ctx, t, config, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "error creating frr container")
	}

	started := false
	defer func() {
		if !started {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_ = c.Close(cleanupCtx)
		}
	}()

	if err := c.Run(ctx); err != nil {
		return nil, errors.Wrap(err, "error running frr container")
	}

	f := &frrImpl{c: c}

	// Exec-based readiness: FRR logs to syslog rather than stdout/stderr, so
	// docker logs is empty. A successful `show version` via vtysh proves the
	// management plane is up; it is daemon-agnostic, so it also works for
	// configs that run a single daemon (e.g. ospfd-only).
	if err := wait.Wait(ctx, c, wait.ForCommand([]string{"vtysh", "-c", "show version"})); err != nil {
		return nil, errors.Wrap(err, "error waiting for frr readiness")
	}

	// `show version` succeeds before a freshly-enabled daemon (e.g. bgpd) has
	// finished starting. Wait for each daemon the caller enabled to appear in
	// `show daemons` so the returned wrapper can be used immediately.
	if err := waitForDaemons(ctx, c, cfg.daemons); err != nil {
		return nil, errors.Wrap(err, "error waiting for frr daemons")
	}

	started = true
	return f, nil
}

// waitForDaemons blocks until every requested daemon appears in `show daemons`.
// vtysh/management-plane failures and a daemon that has not started yet are
// treated as transient (retried), so the wait is robust against daemon startup
// races regardless of which protocol daemons are enabled.
func waitForDaemons(ctx context.Context, c docker.Container, daemons []string) error {
	strategy := func(ctx context.Context, _ wait.Target) (bool, error) {
		res, err := c.Exec(ctx, []string{"vtysh", "-c", "show daemons"})
		if err != nil || res.ExitCode != 0 {
			return false, nil // not ready yet — retry
		}

		running := make(map[string]bool, 8)
		for _, f := range strings.Fields(string(res.Stdout)) {
			running[f] = true
		}
		for _, d := range daemons {
			if !running[d] {
				return false, nil
			}
		}
		return true, nil
	}

	return wait.Wait(ctx, c, strategy)
}

// runVTY runs a command through FRR's vtysh and returns its stdout.
func (f *frrImpl) runVTY(ctx context.Context, cmd string) ([]byte, error) {
	res, err := f.c.Exec(ctx, []string{"vtysh", "-c", cmd})
	if err != nil {
		return nil, errors.Wrap(err, "error exec vtysh")
	}
	if res.ExitCode != 0 {
		return nil, errors.Errorf("vtysh %q exited %d: %s", cmd, res.ExitCode, string(res.Stderr))
	}
	return res.Stdout, nil
}

func (f *frrImpl) Container() docker.Container {
	return f.c
}

func (f *frrImpl) ExecVTY(ctx context.Context, cmd string) ([]byte, error) {
	return f.runVTY(ctx, cmd)
}

// BGPRoutes returns the "routes" key of `show bgp ipv4 unicast json`:
// a map of prefix to the raw JSON route data.
func (f *frrImpl) BGPRoutes(ctx context.Context) (map[string]json.RawMessage, error) {
	out, err := f.runVTY(ctx, "show bgp ipv4 unicast json")
	if err != nil {
		return nil, err
	}

	var data struct {
		Routes map[string]json.RawMessage `json:"routes"`
	}
	if err := json.Unmarshal(out, &data); err != nil {
		return nil, errors.Wrap(err, "error unmarshalling bgp routes json")
	}

	log.Tracef("bgp routes: %d entries", len(data.Routes))

	return data.Routes, nil
}

func (f *frrImpl) HasBGPRoute(ctx context.Context, prefix string) (bool, error) {
	routes, err := f.BGPRoutes(ctx)
	if err != nil {
		return false, err
	}

	_, ok := routes[prefix]
	return ok, nil
}

func (f *frrImpl) MustHasBGPRoute(ctx context.Context, prefix string) bool {
	ok, err := f.HasBGPRoute(ctx, prefix)
	if err != nil {
		panic(err)
	}
	return ok
}

// waitForBGPRoute polls HasBGPRoute until it matches want (true = present,
// false = withdrawn). When the caller's ctx has no deadline, it is bounded by
// defaultWaitTimeout. Extra wait.Options (e.g. WithInterval) are passed
// through.
func (f *frrImpl) waitForBGPRoute(ctx context.Context, prefix string, want bool, opts ...wait.Option) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultWaitTimeout)
		defer cancel()
	}

	strategy := func(ctx context.Context, _ wait.Target) (bool, error) {
		has, err := f.HasBGPRoute(ctx, prefix)
		if err != nil {
			return false, err
		}
		return has == want, nil
	}

	log.Debugf("waiting for bgp route %q present=%v", prefix, want)

	return wait.Wait(ctx, f.c, strategy, opts...)
}

func (f *frrImpl) WaitForBGPRoute(ctx context.Context, prefix string, opts ...wait.Option) error {
	return f.waitForBGPRoute(ctx, prefix, true, opts...)
}

func (f *frrImpl) WaitForBGPRouteWithdrawn(ctx context.Context, prefix string, opts ...wait.Option) error {
	return f.waitForBGPRoute(ctx, prefix, false, opts...)
}

// peerStateJSON mirrors the per-peer object of `show bgp summary json`.
// remoteAs and pfxRcd are emitted as numbers by most FRR versions but as
// strings by some, so they are parsed flexibly.
type peerStateJSON struct {
	RemoteAS         json.RawMessage `json:"remoteAs"`
	State            string          `json:"state"`
	PrefixesReceived json.RawMessage `json:"pfxRcd"`
}

// parseUint coerces a JSON number-or-string into a uint32.
func parseUint(raw json.RawMessage) (uint32, error) {
	var n uint32
	if err := json.Unmarshal(raw, &n); err == nil {
		return n, nil
	}

	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return 0, errors.Wrap(err, "cannot parse numeric field")
	}
	v, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0, errors.Wrap(err, "cannot parse numeric field")
	}
	return uint32(v), nil
}

// BGPSummary returns the parsed `show bgp summary json` output.
func (f *frrImpl) BGPSummary(ctx context.Context) (*BGPSummary, error) {
	out, err := f.runVTY(ctx, "show bgp summary json")
	if err != nil {
		return nil, err
	}

	// FRR 9/10.x nests each address family's summary under a key named after
	// the family (e.g. "ipv4Unicast"), while older versions emit the fields at
	// the top level. Both forms are parsed so the wrapper works across FRR
	// releases; the IPv4 unicast family is used when present.
	var data struct {
		RouterID string                   `json:"routerId"`
		As       json.RawMessage          `json:"as"`
		Peers    map[string]peerStateJSON `json:"peers"`

		IPv4Unicast *struct {
			RouterID string                   `json:"routerId"`
			As       json.RawMessage          `json:"as"`
			Peers    map[string]peerStateJSON `json:"peers"`
		} `json:"ipv4Unicast"`
	}
	if err := json.Unmarshal(out, &data); err != nil {
		return nil, errors.Wrap(err, "error unmarshalling bgp summary json")
	}

	if data.IPv4Unicast != nil {
		data.RouterID = data.IPv4Unicast.RouterID
		data.As = data.IPv4Unicast.As
		data.Peers = data.IPv4Unicast.Peers
	}

	localAS, err := parseUint(data.As)
	if err != nil {
		return nil, errors.Wrap(err, "error parsing local AS")
	}

	peers := make([]PeerState, 0, len(data.Peers))
	for addr, p := range data.Peers {
		remoteAS, err := parseUint(p.RemoteAS)
		if err != nil {
			return nil, errors.Wrapf(err, "error parsing remote AS of peer %s", addr)
		}

		pfxRcd, err := parseUint(p.PrefixesReceived)
		if err != nil {
			return nil, errors.Wrapf(err, "error parsing prefixes received of peer %s", addr)
		}

		peers = append(peers, PeerState{
			Addr:             addr,
			RemoteAS:         remoteAS,
			State:            p.State,
			PrefixesReceived: pfxRcd,
		})
	}

	// data.Peers is a map, so iterating it above yields peers in non-deterministic
	// order. Sort by Addr so Peers is stable across calls.
	sort.Slice(peers, func(i, j int) bool {
		return peers[i].Addr < peers[j].Addr
	})

	return &BGPSummary{
		RouterID:   data.RouterID,
		LocalAS:    localAS,
		Peers:      peers,
		TotalPeers: len(peers),
	}, nil
}

func (f *frrImpl) Peers(ctx context.Context) ([]PeerState, error) {
	s, err := f.BGPSummary(ctx)
	if err != nil {
		return nil, err
	}
	return s.Peers, nil
}

func (f *frrImpl) Close(ctx context.Context) error {
	return f.c.Close(ctx)
}
