# applications/frr

A typed wrapper around an [FRR (FRRouting)](https://frrouting.org/) Docker
container for integration testing. FRR is an independent, C-based BGP
implementation.

The wrapper is intended for end-to-end tests of a BGP-speaking service (e.g.
the `anycastd` project, which uses GoBGP): a test can announce a prefix and
assert that an **independent** BGP peer (this FRR container) receives it, and
that the route is **withdrawn** when the service goes down.

## Image

The image is hosted on **Quay.io**:

- `quay.io/frrouting/frr:10.7.1` (default, `images.FRR`)
- `quay.io/frrouting/frr:10.4.5`

Official FRR images moved to Quay.io; Docker Hub stops at v8.4.1. The pinned
tags are on the current 10.x stable line and are multi-arch.

## Config injection

Three config files are injected at construction via `WithFiles` /
`FileFromBytes`, all as `root:root` (Uid/Gid `0`) with mode `0644`:

| Destination           | Content                                                                                                 |
| --------------------- | ------------------------------------------------------------------------------------------------------- |
| `/etc/frr/daemons`    | `zebra` + `mgmtd` always `yes`; `bgpd` (and any daemons enabled via `WithDaemons`) `yes`, the rest `no` |
| `/etc/frr/vtysh.conf` | `service integrated-vtysh-config`                                                                       |
| `/etc/frr/frr.conf`   | the primary artifact — the injected protocol configuration                                              |

By default only `bgpd` runs (plus the always-on `zebra` and `mgmtd`). Enable
additional protocol daemons with `WithDaemons`:

```go
// Enable BGP + OSPF:
app, err := frr.NewWithConfig(ctx, ospfCfg, frr.WithDaemons("bgpd", "ospfd"))
```

The known-daemon set (in file order) is `zebra, mgmtd, bgpd, ospfd, ospf6d,
ripd, ripngd, isisd, pimd, ldpd, nhrpd, eigrpd, babeld, sharpd, pbrd, staticd,
bfdd, fabricd, vrrpd, pathd`. An unknown daemon name is a build-time error.
`zebra` and `mgmtd` are always force-enabled by the image.

The no-config constructors (`New`, `NewWithImage`, ...) inject a default
`frr.conf`:

```text
hostname frr
frr defaults traditional
```

Use `NewWithConfig(ctx, config)` to inject an arbitrary `frr.conf`, typically
declaring a BGP instance, a router-id and the peers to announce to.

## Readiness

Readiness is **Exec-based**: `wait.ForCommand([]string{"vtysh", "-c", "show version"})`,
followed by a wait for every daemon enabled via `WithDaemons` to appear in
`show daemons`. FRR logs to syslog rather than stdout/stderr, so `docker logs`
is empty and a log-line matcher cannot be used. `show version` is
daemon-agnostic (so it also works for ospfd-only configs), and the daemon wait
ensures a freshly-enabled daemon has finished starting before the wrapper is
returned.

## Privileged mode

FRR needs to manipulate routes (zebra installs routes in the kernel via
netlink), so the container runs with `docker.WithPrivileged()` (as the k3s
wrapper does).

**Port 179 is not exposed to the host.** BGP peering happens between containers
on the docker network, not via host DNAT. To peer two FRR instances, put them
on a shared `docker.Group` internal network and configure each with the other's
container-name alias as the peer.

**Least-privilege alternative**: if your runner can provide a least-privilege
setup, you may instead grant only the needed capabilities with
`docker.WithCapAdd("NET_ADMIN", "NET_BIND_SERVICE")` instead of full
privileged mode. The wrapper defaults to `WithPrivileged()` for maximum
compatibility; override via `WithHostConfig` when constructing the container.

## Group usage (multiple FRR peers)

For end-to-end tests with two real FRR BGP peers (or an FRR peer talking to a
service container), build unstarted containers with `NewContainer` /
`NewContainerT`, put them in a `docker.Group`, and wrap the started containers
back with `NewFromContainer`:

```go
cA, _ := frr.NewContainer(ctx, cfgA, frr.WithContainerName("frr-a"))
cB, _ := frr.NewContainer(ctx, cfgB, frr.WithContainerName("frr-b"))

g, _ := docker.NewGroup("frr-peer", docker.NewApplication(cA), docker.NewApplication(cB))
defer g.Close(ctx)
g.Run(ctx)

appA := frr.NewFromContainer(cA)
appB := frr.NewFromContainer(cB)
```

- `NewContainer` / `NewContainerT` only **build** the container; the Group's
  `Run()` starts it. `NewFromContainer` wraps a running container as the typed
  `FRR` interface.
- `WithContainerName(name)` sets the container name / DNS alias on the group's
  internal network (default `"frr"`). Two peers must use distinct names so they
  can resolve each other by name.
- The Group uses an **internal** network, so peers resolve each other by
  container-name alias with no host port.

> **FRR eBGP note**: FRR 9.x+ enables `bgp ebgp-requires-policy` by default,
> which holds an eBGP peer in "Policy" state and exchanges **zero** prefixes
> unless a route policy is configured. Add `no bgp ebgp-requires-policy` to the
> BGP config to exchange prefixes without a route-map/prefix-list. Also note
> FRR does not auto-resolve a hostname BGP neighbor — resolve the peer's
> container-name alias to its IP on the internal network (e.g. `getent hosts
<name>`) and configure the neighbor by that IP. See
> `applications/frr/frr_group_test.go` for a complete two-peer example.

## Usage

```go
app, err := frr.NewWithConfig(ctx, config)
if err != nil {
    // handle error
}
defer func() { _ = app.Close(ctx) }()

ok, err := app.HasBGPRoute(ctx, "10.0.0.0/24")
if err != nil {
    // handle error
}

// Or wait for a route to appear / be withdrawn:
if err := app.WaitForBGPRoute(ctx, "10.0.0.0/24"); err != nil {
    // handle error
}
if err := app.WaitForBGPRouteWithdrawn(ctx, "10.0.0.0/24"); err != nil {
    // handle error
}

peers, err := app.Peers(ctx)
summary, err := app.BGPSummary(ctx)
```

The interface exposes `Container()`, `ExecVTY`, `BGPRoutes`,
`HasBGPRoute` / `MustHasBGPRoute`, `WaitForBGPRoute` /
`WaitForBGPRouteWithdrawn`, `Peers`, `BGPSummary` and `Close`.

## Versions tested

- `quay.io/frrouting/frr:10.7.1`
- `quay.io/frrouting/frr:10.4.5`

## Gotchas

- **Logs are empty**: FRR writes to syslog, so never wait on `docker logs`
  for readiness or assertions — use `ExecVTY`/`Exec` instead.
- **Privileged is required**: without `WithPrivileged()`, installing routes
  fails; the container will not come up ready.
- **vtysh config**: `service integrated-vtysh-config` is required so vtysh
  reads the injected `frr.conf`.
- **`bgp ebgp-requires-policy`**: FRR 9.x+ holds eBGP peers in "Policy" state
  with zero prefixes exchanged unless a policy is configured. Add
  `no bgp ebgp-requires-policy` (or configure a route policy) to exchange
  prefixes.
- **`pfxRcd` / `remoteAs` are number-or-string**: some FRR versions emit
  these as strings in JSON; the wrapper parses both.
- **JSON table location**: routes live under the top-level `"routes"` key of
  `show bgp ipv4 unicast json` (a `map[prefix]raw`); peers live under the
  `"peers"` key of `show bgp summary json`.
