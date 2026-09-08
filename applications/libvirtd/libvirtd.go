// Package libvirtd runs libvirtd (the KVM/QEMU virtualization manager) in a
// Docker container and returns a connected go-libvirt client. This exposes the
// full libvirt API — virtual machine (domain) lifecycle, storage pools and
// volumes, virtual networks, snapshots and more — so tests can drive real
// virtualization without mocks.
//
// The container is configured to listen on a TCP socket (port 16509) so the
// test connects over TCP. By default it runs in a least-privilege
// configuration: every capability is dropped and only the minimal set
// QEMU/libvirtd need is added back, with Docker's default seccomp profile
// relaxed (seccomp=unconfined) so QEMU can start. /dev/kvm and /dev/net/tun
// are passed through when they exist on the host (otherwise QEMU falls back to
// software/TCG emulation). Call WithPrivileged for full-privilege mode.
package libvirtd

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/digitalocean/go-libvirt"
	"github.com/digitalocean/go-libvirt/socket/dialers"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/teran/go-docker-testsuite"
	"github.com/teran/go-docker-testsuite/images"
)

const (
	// libvirtTCPPort is the port libvirtd listens on for TCP connections.
	libvirtTCPPort = 16509
	// libvirtdConfigPath is the path of libvirtd.conf inside the container.
	libvirtdConfigPath = "/etc/libvirt/libvirtd.conf"
	// libvirtdConfig is the configuration we inject to enable TCP listening
	// without authentication (suitable for ephemeral test containers).
	libvirtdConfig = "listen_tcp = 1\nauth_tcp = \"none\"\n"
	// pollInterval is how often to poll for readiness.
	pollInterval = 500 * time.Millisecond
	// connectAttemptTimeout bounds a single go-libvirt Connect() attempt, which
	// can otherwise block indefinitely while libvirtd is still starting.
	connectAttemptTimeout = 5 * time.Second
	// disconnectTimeout bounds a single go-libvirt Disconnect() attempt, which
	// can otherwise block on its RPC response channel when libvirtd is
	// unresponsive.
	disconnectTimeout = 5 * time.Second
)

// libvirtCaps is the least-privilege capability set QEMU/libvirtd need for
// domain lifecycle, file/dir storage pools & volumes, and virtual networks.
// SYS_ADMIN (mount/loop, LVM pools) is deliberately excluded — enable it via
// WithPrivileged if a workload needs it.
var libvirtCaps = []string{
	"NET_ADMIN",        // create/manage bridges, taps, iptables/nftables NAT for virtual networks
	"NET_RAW",          // raw sockets (ARP/ebtables) used by virtual-network handling
	"NET_BIND_SERVICE", // dnsmasq binds low ports (DHCP 67, DNS 53) on virtual networks
	"DAC_OVERRIDE",     // read disk-image and pool files regardless of host permissions
	"DAC_READ_SEARCH",  // read pool dirs/files when QEMU runs as a non-root user
	"SYS_NICE",         // QEMU vCPU thread priority/scheduling
	"SYS_RESOURCE",     // raise rlimits and mlock guest RAM
	"SYS_PTRACE",       // QEMU monitor/GDB and thread control
	"MKNOD",            // create device nodes for loop/storage pools
	"CHOWN",            // chown pool image files
	"SETUID",           // QEMU drops to a non-root user (libvirt-qemu)
	"SETGID",
	"FOWNER", // chmod/setuid-bit on pool files
	"FSETID",
	"KILL",
	"SETPCAP",
	"IPC_LOCK",    // mlock guest RAM
	"AUDIT_WRITE", // write audit records
}

// Option configures a libvirtd container.
type Option func(*options)

type options struct {
	privileged bool
}

// WithPrivileged runs the container in full Docker privileged mode. This is
// an opt-in for workloads that need capabilities beyond the least-privilege
// default (e.g. SYS_ADMIN for mount/loop-backed storage pools or LVM). It is
// mutually exclusive with the default capability whitelist.
func WithPrivileged() Option {
	return func(o *options) {
		o.privileged = true
	}
}

// Libvirt is the interface returned by the libvirtd application.
type Libvirt interface {
	// Client returns a connected go-libvirt client exposing the full
	// libvirt API (domains, storage pools/volumes, networks, snapshots, ...).
	Client() *libvirt.Libvirt
	// Addr returns the host:port of the libvirtd TCP endpoint.
	Addr() string
	// HasKVM reports whether /dev/kvm was present and passed into the
	// container (i.e. whether KVM acceleration is in use vs software/TCG).
	HasKVM() bool
	Close(ctx context.Context) error
}

type libvirtd struct {
	c      docker.Container
	addr   string
	client *libvirt.Libvirt
	hasKVM bool
}

// New starts a libvirtd container using the default image.
func New(ctx context.Context, opts ...Option) (Libvirt, error) {
	return NewWithImage(ctx, images.Libvirtd, opts...)
}

// NewWithImage starts a libvirtd container using the given image.
//
// libvirtd is configured to listen on TCP (port 16509) via an injected
// libvirtd.conf, and the port is exposed to the host. /dev/kvm and
// /dev/net/tun are passed through only if they exist on the host; when
// /dev/kvm is absent, QEMU falls back to slower software (TCG) emulation
// rather than failing.
func NewWithImage(ctx context.Context, image string, opts ...Option) (Libvirt, error) {
	o := options{}
	for _, opt := range opts {
		opt(&o)
	}

	cfgDir, err := os.MkdirTemp("", "libvirtd-*")
	if err != nil {
		return nil, errors.Wrap(err, "error creating libvirtd config directory")
	}

	cfgPath := filepath.Join(cfgDir, "libvirtd.conf")
	if err := os.WriteFile(cfgPath, []byte(libvirtdConfig), 0o600); err != nil {
		_ = os.RemoveAll(cfgDir)
		return nil, errors.Wrap(err, "error writing libvirtd.conf")
	}

	containerOpts := []docker.ContainerOption{
		docker.WithBinds(cfgPath + ":" + libvirtdConfigPath + ":ro"),
	}

	// Devices are mounted only when they exist on the host. A missing /dev/kvm
	// silently downgrades to TCG software emulation (never fails container
	// creation); a missing /dev/net/tun means user-mode (SLIRP) networking only.
	hasKVM := hostDeviceExists("/dev/kvm", image)
	hasTUN := hostDeviceExists("/dev/net/tun", image)

	var devices []string
	if hasKVM {
		devices = append(devices, "/dev/kvm:/dev/kvm:rw")
	}
	if hasTUN {
		devices = append(devices, "/dev/net/tun:/dev/net/tun:rw")
	}
	if len(devices) > 0 {
		containerOpts = append(containerOpts, docker.WithDevices(devices...))
	}

	if o.privileged {
		containerOpts = append(containerOpts, docker.WithPrivileged())
	} else {
		containerOpts = append(containerOpts,
			docker.WithCapDrop("ALL"),
			docker.WithCapAdd(libvirtCaps...),
			// Keep capabilities minimal but relax seccomp: QEMU needs syscalls
			// blocked by Docker's default profile to start reliably.
			docker.WithSecurityOpt("seccomp=unconfined"),
		)
	}

	c, err := docker.NewContainer(
		"libvirtd",
		image,
		nil,
		docker.NewEnvironment(),
		docker.NewPortBindings().PortDNAT(docker.ProtoTCP, libvirtTCPPort),
		containerOpts...,
	)
	if err != nil {
		_ = os.RemoveAll(cfgDir)
		return nil, err
	}

	started := false
	defer func() {
		if !started {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_ = c.Close(cleanupCtx)
			_ = os.RemoveAll(cfgDir)
		}
	}()

	if err := c.Run(ctx); err != nil {
		return nil, err
	}

	hp, err := c.URL(docker.ProtoTCP, libvirtTCPPort)
	if err != nil {
		return nil, err
	}

	addr := hp.String()

	// Readiness: the host TCP port (Docker's userland proxy) accepts
	// connections as soon as the container starts, before libvirtd has bound
	// the port inside. So a plain TCP dial is not a reliable readiness probe —
	// retry the libvirt RPC connect (with a fresh client per attempt) until
	// libvirtd actually answers the protocol.
	var client *libvirt.Libvirt
	if err := connectWithRetry(ctx, hp, func(c *libvirt.Libvirt) error {
		client = c
		return nil
	}); err != nil {
		return nil, err
	}

	started = true
	return &libvirtd{
		c:      c,
		addr:   addr,
		client: client,
		hasKVM: hasKVM,
	}, nil
}

// connectWithRetry dials libvirtd over TCP and completes the go-libvirt
// handshake, retrying with a fresh dialer until it succeeds or ctx is done.
//
// go-libvirt's Connect() can block indefinitely: its RPC waits on a response
// channel, and when libvirtd is listening but not yet serving the protocol it
// never fills that channel (the host TCP port / Docker proxy accepts
// connections before libvirtd binds inside). Each attempt is therefore bounded
// by connectAttemptTimeout so a slow startup cannot hang the test.
func connectWithRetry(ctx context.Context, hp *docker.HostPort, onSuccess func(*libvirt.Libvirt) error) error {
	var lastErr error
	for {
		client := libvirt.NewWithDialer(dialers.NewRemote(hp.Host, dialers.UsePort(strconv.Itoa(int(hp.Port)))))

		attemptCtx, cancel := context.WithTimeout(ctx, connectAttemptTimeout)
		errCh := make(chan error, 1)
		go func() { errCh <- client.Connect() }()

		var err error
		select {
		case err = <-errCh:
		case <-attemptCtx.Done():
			err = attemptCtx.Err()
		}
		cancel()

		if err == nil {
			return onSuccess(client)
		}
		lastErr = err

		select {
		case <-ctx.Done():
			return errors.Wrap(lastErr, "error connecting to libvirtd")
		case <-time.After(pollInterval):
		}
	}
}

// hostDeviceExists reports whether a host device path exists, logging at
// debug level when it does not (library-internal severity per AGENTS.md).
func hostDeviceExists(path, image string) bool {
	if _, err := os.Stat(path); err == nil {
		return true
	}
	log.WithFields(log.Fields{
		"device": path,
		"image":  image,
	}).Debug("host device not found; skipping passthrough")
	return false
}

func (l *libvirtd) Client() *libvirt.Libvirt { return l.client }

func (l *libvirtd) Addr() string { return l.addr }

func (l *libvirtd) HasKVM() bool { return l.hasKVM }

func (l *libvirtd) Close(ctx context.Context) error {
	var err error
	if l.client != nil {
		// Bound Disconnect: go-libvirt's Disconnect can block on its RPC
		// response channel when libvirtd is unresponsive, which would hang
		// cleanup. A bounded attempt is better than a deadlock in a test.
		dc, cancel := context.WithTimeout(ctx, disconnectTimeout)
		errCh := make(chan error, 1)
		go func() { errCh <- l.client.Disconnect() }()
		select {
		case err = <-errCh:
		case <-dc.Done():
			err = dc.Err()
		}
		cancel()
	}
	if cerr := l.c.Close(ctx); cerr != nil && err == nil {
		err = cerr
	}
	return err
}
