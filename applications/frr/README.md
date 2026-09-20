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

| Destination            | Content                                                        |
| ---------------------- | -------------------------------------------------------------- |
| `/etc/frr/daemons`     | enables `zebra` and `bgpd`, disables every other daemon        |
| `/etc/frr/vtysh.conf`  | `service integrated-vtysh-config`                              |
| `/etc/frr/frr.conf`    | the primary artifact — the injected BGP configuration         |

The no-config constructors (`New`, `NewWithImage`, ...) inject a default
`frr.conf`:

```
hostname frr
frr defaults traditional
```

Use `NewWithConfig(ctx, config)` to inject an arbitrary `frr.conf`, typically
declaring a BGP instance, a router-id and the peers to announce to.

## Readiness

Readiness is **Exec-based**: `wait.ForCommand([]string{"vtysh", "-c", "show bgp summary"})`.
FRR logs to syslog rather than stdout/stderr, so `docker logs` is empty and a
log-line matcher cannot be used. A successful `show bgp summary` via vtysh
proves the daemons are up and vtysh can reach them.

## Privileged mode

FRR needs to bind TCP port 179 and manipulate routes (zebra installs routes in
the kernel via netlink), so the container runs with `docker.WithPrivileged()`
(as the k3s wrapper does) and DNATs TCP/179.

**Least-privilege alternative**: if your runner can provide a least-privilege
setup, you may instead grant only the needed capabilities with
`docker.WithCapAdd("NET_ADMIN", "NET_BIND_SERVICE")` instead of full
privileged mode. The wrapper defaults to `WithPrivileged()` for maximum
compatibility; override via `WithHostConfig` when constructing the container.

**Security note**: TCP/179 is host-DNAT-mapped onto the container, so the
wrapper is intended for **trusted hosts / ephemeral CI runners** only. Do not
run BGP containers that bind privileged ports on an untrusted or shared host.

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
- **Privileged is required**: without `WithPrivileged()`, binding port 179
  and installing routes fails; the container will not come up ready.
- **vtysh config**: `service integrated-vtysh-config` is required so vtysh
  reads the injected `frr.conf`.
- **`pfxRcd` / `remoteAs` are number-or-string**: some FRR versions emit
  these as strings in JSON; the wrapper parses both.
- **JSON table location**: routes live under the top-level `"routes"` key of
  `show bgp ipv4 unicast json` (a `map[prefix]raw`); peers live under the
  `"peers"` key of `show bgp summary json`.
