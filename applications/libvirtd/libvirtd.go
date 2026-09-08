// Package libvirtd runs libvirtd (the KVM/QEMU virtualization manager) in a
// Docker container and returns a connected go-libvirt client. This exposes the
// full libvirt API — virtual machine (domain) lifecycle, storage pools and
// volumes, virtual networks, snapshots and more — so tests can drive real
// virtualization without mocks.
//
// By default the container runs in a least-privilege configuration: every
// capability is dropped and only the minimal set QEMU/libvirtd need is added
// back, with Docker's default seccomp profile relaxed (seccomp=unconfined) so
// QEMU can start. /dev/kvm and /dev/net/tun are passed through when they exist
// on the host (otherwise QEMU falls back to software/TCG emulation). Call
// WithPrivileged for a full-privilege mode when a workload needs it.
package libvirtd

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/digitalocean/go-libvirt"
	"github.com/digitalocean/go-libvirt/socket/dialers"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/teran/go-docker-testsuite"
	"github.com/teran/go-docker-testsuite/images"
)

const (
	// libvirtSocketDir is the path inside the container where libvirtd
	// creates its unix control socket.
	libvirtSocketDir = "/var/run/libvirt"
	// libvirtSocketName is the name of the system (root) libvirt socket
	// created inside libvirtSocketDir.
	libvirtSocketName = "libvirt-sock"
	// socketPollInterval is how often to poll for the socket file appearing.
	socketPollInterval = 500 * time.Millisecond
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
	// SocketPath returns the path to the libvirtd unix socket on the host.
	SocketPath() string
	// HasKVM reports whether /dev/kvm was present and passed into the
	// container (i.e. whether KVM acceleration is in use vs software/TCG).
	HasKVM() bool
	Close(ctx context.Context) error
}

type libvirtd struct {
	c        docker.Container
	sockDir  string
	sockPath string
	client   *libvirt.Libvirt
	hasKVM   bool
}

// New starts a libvirtd container using the default image.
func New(ctx context.Context, opts ...Option) (Libvirt, error) {
	return NewWithImage(ctx, images.Libvirtd, opts...)
}

// NewWithImage starts a libvirtd container using the given image.
//
// The container is started in a least-privilege configuration by default
// (see the package doc). /dev/kvm and /dev/net/tun are passed through only if
// they exist on the host; when /dev/kvm is absent, QEMU falls back to slower
// software (TCG) emulation rather than failing.
func NewWithImage(ctx context.Context, image string, opts ...Option) (Libvirt, error) {
	o := options{}
	for _, opt := range opts {
		opt(&o)
	}

	sockDir, err := os.MkdirTemp("", "libvirtd-*")
	if err != nil {
		return nil, errors.Wrap(err, "error creating libvirtd socket directory")
	}

	containerOpts := []docker.ContainerOption{
		docker.WithBinds(sockDir + ":" + libvirtSocketDir),
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
		docker.NewPortBindings(),
		containerOpts...,
	)
	if err != nil {
		_ = os.RemoveAll(sockDir)
		return nil, err
	}

	started := false
	defer func() {
		if !started {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_ = c.Close(cleanupCtx)
			_ = os.RemoveAll(sockDir)
		}
	}()

	if err := c.Run(ctx); err != nil {
		return nil, err
	}

	sockPath := filepath.Join(sockDir, libvirtSocketName)
	if err := waitForSocket(ctx, sockPath); err != nil {
		return nil, err
	}

	client := libvirt.NewWithDialer(dialers.NewLocal(dialers.WithSocket(sockPath)))
	if err := client.Connect(); err != nil {
		return nil, errors.Wrap(err, "error connecting to libvirtd")
	}

	started = true
	return &libvirtd{
		c:        c,
		sockDir:  sockDir,
		sockPath: sockPath,
		client:   client,
		hasKVM:   hasKVM,
	}, nil
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

// waitForSocket polls until the libvirtd unix socket file exists or ctx is done.
func waitForSocket(ctx context.Context, path string) error {
	for {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(socketPollInterval):
		}
	}
}

func (l *libvirtd) Client() *libvirt.Libvirt { return l.client }

func (l *libvirtd) SocketPath() string { return l.sockPath }

func (l *libvirtd) HasKVM() bool { return l.hasKVM }

func (l *libvirtd) Close(ctx context.Context) error {
	var err error
	if l.client != nil {
		err = l.client.Disconnect()
	}
	if cerr := l.c.Close(ctx); cerr != nil && err == nil {
		err = cerr
	}
	if rerr := os.RemoveAll(l.sockDir); rerr != nil && err == nil {
		err = rerr
	}
	return err
}
