package docker

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/distribution/reference"
	dockerContainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-units"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/teran/go-docker-testsuite/internal/ptr"
)

const (
	// defaultStopTimeout bounds the graceful stop window passed to Docker's
	// ContainerStop. Integration-test containers are ephemeral and often do not
	// terminate on SIGTERM (e.g. busybox sleep); Docker force-kills them once
	// the timeout elapses, so keeping this short avoids a 60s+ stall on every
	// container Close. Services that need a graceful shutdown are better served
	// by a container hook than by an unbounded stop timeout.
	defaultStopTimeout = 3 * time.Second

	// defaultExecTimeout bounds a single in-container lifecycle command when
	// the caller's context carries no deadline. Without it a hung exec would
	// block Run() indefinitely.
	defaultExecTimeout = 1 * time.Minute
)

var errImageIsNotPulled = errors.New("image is not pulled")

type (
	ContainerID = string
	NetworkID   = string
)

// ContainerOption modifies the docker HostConfig before container creation.
type ContainerOption func(*dockerContainer.HostConfig)

// WithPrivileged grants the container elevated privileges.
func WithPrivileged() ContainerOption {
	return func(hc *dockerContainer.HostConfig) {
		hc.Privileged = true
	}
}

// WithTmpfs mounts tmpfs filesystems at the given paths.
func WithTmpfs(m map[string]string) ContainerOption {
	return func(hc *dockerContainer.HostConfig) {
		hc.Tmpfs = m
	}
}

// WithBinds adds volume bind mounts (host:container[:mode]).
func WithBinds(binds ...string) ContainerOption {
	return func(hc *dockerContainer.HostConfig) {
		hc.Binds = binds
	}
}

// WithUlimit sets an ulimit (e.g. nofile) on the container's HostConfig.
// Some images (e.g. Ceph) start much slower under Docker's default limits,
// so callers can raise the soft/hard limit.
func WithUlimit(name string, soft, hard int64) ContainerOption {
	return func(hc *dockerContainer.HostConfig) {
		hc.Ulimits = append(hc.Ulimits, &units.Ulimit{
			Name: name,
			Soft: soft,
			Hard: hard,
		})
	}
}

// WithDevices maps host devices into the container (e.g. "/dev/kvm",
// "/dev/net/tun"). Each entry is "hostPath" or "hostPath:containerPath[:mode]".
// This is required for workloads that need direct access to host devices,
// such as KVM/QEMU virtualization (libvirtd) or TUN/TAP networking.
func WithDevices(devices ...string) ContainerOption {
	return func(hc *dockerContainer.HostConfig) {
		for _, d := range devices {
			parts := strings.SplitN(d, ":", 3)
			dd := &dockerContainer.DeviceMapping{
				PathOnHost:        parts[0],
				PathInContainer:   parts[0],
				CgroupPermissions: "rwm",
			}
			if len(parts) > 1 && parts[1] != "" {
				dd.PathInContainer = parts[1]
			}
			if len(parts) > 2 && parts[2] != "" {
				dd.CgroupPermissions = parts[2]
			}
			hc.Devices = append(hc.Devices, *dd)
		}
	}
}

// WithCapAdd grants additional Linux capabilities (e.g. "NET_ADMIN",
// "SYS_NICE") beyond the Docker default set. Values are capability names
// without the "CAP_" prefix, as accepted by Docker.
func WithCapAdd(caps ...string) ContainerOption {
	return func(hc *dockerContainer.HostConfig) {
		hc.CapAdd = append(hc.CapAdd, caps...)
	}
}

// WithCapDrop removes Linux capabilities from the container's default set
// for least-privilege hardening. Values are capability names without the
// "CAP_" prefix. Passing "ALL" drops every capability; combine with
// WithCapAdd to whitelist only what is needed.
func WithCapDrop(caps ...string) ContainerOption {
	return func(hc *dockerContainer.HostConfig) {
		hc.CapDrop = append(hc.CapDrop, caps...)
	}
}

// WithSecurityOpt sets Docker security options (e.g. "seccomp=unconfined",
// "apparmor=unconfined"). Needed by workloads such as QEMU/libvirtd whose
// syscalls may be blocked by Docker's default seccomp profile when running
// in a least-privilege (non-privileged) configuration.
func WithSecurityOpt(opts ...string) ContainerOption {
	return func(hc *dockerContainer.HostConfig) {
		hc.SecurityOpt = append(hc.SecurityOpt, opts...)
	}
}

// WithMemoryLimit caps the container's memory usage (bytes) via
// HostConfig.Memory.
func WithMemoryLimit(bytes int64) ContainerOption {
	return func(hc *dockerContainer.HostConfig) {
		hc.Memory = bytes
	}
}

// WithMemoryReservation sets a soft memory reservation (bytes) via
// HostConfig.MemoryReservation.
func WithMemoryReservation(bytes int64) ContainerOption {
	return func(hc *dockerContainer.HostConfig) {
		hc.MemoryReservation = bytes
	}
}

// WithMemorySwap sets the swap limit (bytes) via HostConfig.MemorySwap.
// -1 disables swap; a value equal to Memory effectively disables swap;
// typically callers pass 2*Memory to allow double the memory in swap.
func WithMemorySwap(bytes int64) ContainerOption {
	return func(hc *dockerContainer.HostConfig) {
		hc.MemorySwap = bytes
	}
}

// WithCPUs sets the CPU limit as a fractional vCPU count via
// HostConfig.NanoCPUs (e.g. 0.5 → 500000000 nanos). Non-positive values are
// ignored (no-op).
func WithCPUs(count float64) ContainerOption {
	return func(hc *dockerContainer.HostConfig) {
		if count <= 0 {
			return
		}
		hc.NanoCPUs = int64(count * 1e9)
	}
}

// WithCpusetCpus pins the container to specific host CPUs via
// HostConfig.CpusetCpus (e.g. "0-2,7").
func WithCpusetCpus(cpus string) ContainerOption {
	return func(hc *dockerContainer.HostConfig) {
		hc.CpusetCpus = cpus
	}
}

// WithPidsLimit caps the number of processes inside the container via
// HostConfig.PidsLimit.
func WithPidsLimit(limit int64) ContainerOption {
	return func(hc *dockerContainer.HostConfig) {
		hc.PidsLimit = &limit
	}
}

// ParseRAMSize parses a human-readable size string (e.g. "512m", "1g",
// "1.5g") into bytes, wrapping github.com/docker/go-units.RAMInBytes via
// pkg/errors.
func ParseRAMSize(s string) (int64, error) {
	n, err := units.RAMInBytes(s)
	if err != nil {
		return 0, errors.Wrap(err, "error parsing RAM size")
	}
	return n, nil
}

// Container exposes interface to control the container runtime
type Container interface {
	AwaitOutput(ctx context.Context, m Matcher) error
	Close(ctx context.Context) error
	Exec(ctx context.Context, cmd []string) (*ExecResult, error)
	GetOutput(ctx context.Context, m ...Matcher) ([]string, error)
	ID() ContainerID
	Name() string
	NetworkAttach(networkID string) error
	Ping(ctx context.Context) error
	Run(ctx context.Context) error
	URL(proto Protocol, port uint16) (*HostPort, error)
}

// ExecResult carries the captured output and exit status of an Exec call.
type ExecResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// Error returns a non-nil error if the command exited non-zero (includes exit
// code + stderr for diagnostics); nil when ExitCode == 0.
func (r *ExecResult) Error() error {
	if r == nil || r.ExitCode == 0 {
		return nil
	}
	return errors.Errorf("command exited with code %d: %s", r.ExitCode, string(r.Stderr))
}

// Combined returns Stdout followed by Stderr concatenated.
func (r *ExecResult) Combined() []byte {
	var b bytes.Buffer
	b.Write(r.Stdout)
	b.Write(r.Stderr)
	return b.Bytes()
}

type container struct {
	cli *client.Client

	name          string
	image         string
	env           Environment
	cmd           []string
	containerID   ContainerID
	networkID     NetworkID
	ports         *PortBindings
	indirectPorts map[string]string
	containerOpts []ContainerOption

	startupCmd        []string
	afterReadyCmd     []string
	afterReadyMatcher Matcher
}

// New creates new container instance from remote docker image
func NewContainer(name, image string, cmd []string, environment Environment, ports *PortBindings, opts ...ContainerOption) (Container, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}

	return NewContainerWithClient(cli, name, image, cmd, environment, ports, opts...)
}

// NewContainerWithClient creates new container from remote docker image and allows
// to pass custom docker.Client instance
func NewContainerWithClient(cli *client.Client, name, image string, cmd []string, env Environment, ports *PortBindings, opts ...ContainerOption) (Container, error) {
	log.WithFields(log.Fields{
		"name":  name,
		"image": image,
	}).Debugf("initializing container")

	imageRef := image
	prefix := os.Getenv("IMAGE_PREFIX")
	if prefix != "" {
		imageRef = strings.TrimRight(prefix, "/") + "/" + strings.TrimLeft(imageRef, "/")
		log.WithFields(log.Fields{
			"original": image,
			"prefixed": imageRef,
		}).Trace("Setting prefix for image (for proxy purposes since IMAGE_PREFIX is present)")
	}

	return &container{
		cli:           cli,
		name:          name,
		image:         imageRef,
		cmd:           cmd,
		env:           env,
		ports:         ports,
		indirectPorts: make(map[string]string),
		containerOpts: opts,
	}, nil
}

// LifecycleOption configures behavior that runs inside the container during
// Run(), in addition to HostConfig-based ContainerOptions. It does not modify
// the Docker HostConfig.
type LifecycleOption func(*container)

// WithStartupCommand runs cmd inside the container immediately after it
// starts, failing Run() if the command errors or exits non-zero.
func WithStartupCommand(cmd ...string) LifecycleOption {
	return func(c *container) {
		c.startupCmd = cmd
	}
}

// WithAfterReadyCommand runs cmd inside the container once the readiness
// matcher is satisfied by the container output. When ready is nil the command
// runs immediately after the startup command (or right after start if none).
func WithAfterReadyCommand(ready Matcher, cmd ...string) LifecycleOption {
	return func(c *container) {
		c.afterReadyMatcher = ready
		c.afterReadyCmd = cmd
	}
}

// WithHostConfig adapts ordinary ContainerOption values so they can be
// supplied alongside lifecycle options. They modify the Docker HostConfig as
// usual.
func WithHostConfig(opts ...ContainerOption) LifecycleOption {
	return func(c *container) {
		c.containerOpts = append(c.containerOpts, opts...)
	}
}

// NewContainerWithLifecycle is NewContainer plus lifecycle options. Existing
// NewContainer is unchanged.
func NewContainerWithLifecycle(name, image string, cmd []string, environment Environment, ports *PortBindings, opts ...LifecycleOption) (Container, error) {
	c, err := NewContainer(name, image, cmd, environment, ports)
	if err != nil {
		return nil, err
	}

	cc := c.(*container)
	for _, o := range opts {
		o(cc)
	}
	return cc, nil
}

// AwaitOutput blocks the execution for any of (whatever comes first): string matched Matcher or timeout
func (c *container) AwaitOutput(ctx context.Context, m Matcher) error {
	rd, err := c.cli.ContainerLogs(ctx, c.containerID, dockerContainer.LogsOptions{
		ShowStderr: true,
		ShowStdout: true,
		Follow:     true,
	})
	if err != nil {
		return err
	}
	defer func() { _ = rd.Close() }()

	s := bufio.NewScanner(rd)
	for s.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			l := s.Text()

			log.WithFields(log.Fields{
				"line": l,
			}).Tracef("processing log line")

			if m(l) {
				return nil
			}
		}
	}

	return s.Err()
}

func (c *container) GetOutput(ctx context.Context, ms ...Matcher) ([]string, error) {
	rd, err := c.cli.ContainerLogs(ctx, c.containerID, dockerContainer.LogsOptions{
		ShowStderr: true,
		ShowStdout: true,
		Follow:     false,
	})
	if err != nil {
		return nil, err
	}
	defer func() { _ = rd.Close() }()

	out := []string{}
	s := bufio.NewScanner(rd)
	for s.Scan() {
		l := s.Text()

		log.WithFields(log.Fields{
			"line": l,
		}).Tracef("processing log line")

		for _, m := range ms {
			if m(l) {
				out = append(out, l)
			}
		}
	}

	return out, s.Err()
}

// Exec runs a command inside the running container and captures its output and
// exit status. Non-zero exit code is NOT an error — it is reported in the
// returned ExecResult, so callers can assert on failure as easily as success.
// Errors are non-nil only when the exec *infrastructure* fails
// (create/attach/inspect/IO or ctx cancelled). Requires container to be
// running.
func (c *container) Exec(ctx context.Context, cmd []string) (*ExecResult, error) {
	if c.containerID == "" {
		return nil, errors.New("container is not running")
	}

	log.WithFields(log.Fields{
		"container": c.containerID,
		"cmd":       cmd,
	}).Trace("executing command in container via exec")

	execConfig := dockerContainer.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	}

	execResp, err := c.cli.ContainerExecCreate(ctx, c.containerID, execConfig)
	if err != nil {
		return nil, errors.Wrap(err, "error creating exec instance")
	}

	attachResp, err := c.cli.ContainerExecAttach(ctx, execResp.ID, dockerContainer.ExecAttachOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "error attaching to exec")
	}
	defer attachResp.Close()

	// Docker multiplexes stdout and stderr into a single stream with headers.
	// Use stdcopy to demultiplex and capture each stream separately.
	var stdoutBuf, stderrBuf bytes.Buffer
	_, err = stdcopy.StdCopy(&stdoutBuf, &stderrBuf, attachResp.Reader)
	if err != nil {
		return nil, errors.Wrap(err, "error demuxing exec output")
	}

	inspectResp, err := c.cli.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		return nil, errors.Wrap(err, "error inspecting exec result")
	}

	return &ExecResult{
		Stdout:   stdoutBuf.Bytes(),
		Stderr:   stderrBuf.Bytes(),
		ExitCode: inspectResp.ExitCode,
	}, nil
}

func (c *container) Name() string {
	return c.name
}

// ID returns the Docker container ID. It is only available after Run() is called.
func (c *container) ID() ContainerID {
	return c.containerID
}

// NetworkAttach attaches the container to the specific network
// Usable when it's needed to group amount of containers with IP-level connectivity
func (c *container) NetworkAttach(networkID string) error {
	c.networkID = networkID
	return nil
}

// Ping gonna ping (the Docker daemon)
func (c *container) Ping(ctx context.Context) error {
	_, err := c.cli.Ping(ctx)
	return err
}

// Run starts the container
func (c *container) Run(ctx context.Context) error {
	err := c.pullImage(ctx)
	if err != nil {
		return err
	}

	containerConfig := &dockerContainer.Config{
		Image:        c.image,
		Env:          c.env.Eval(newContainerInfoFromContainer(c)),
		Cmd:          c.cmd,
		ExposedPorts: c.ports.portSet(),
		Labels: map[string]string{
			"go-docker-testsuite.name": c.name,
		},
	}

	networkConfig := &network.NetworkingConfig{}

	log.WithFields(log.Fields{
		"ports": c.ports,
	}).Trace("creating new host config ...")

	hostConfig, err := NewHostConfig(c.ports, c.containerOpts...)
	if err != nil {
		return errors.Wrap(err, "error gathering host configuration")
	}

	container, err := c.cli.ContainerCreate(
		ctx,
		containerConfig,
		hostConfig,
		networkConfig,
		nil,
		"",
	)
	if err != nil {
		return errors.Wrap(err, "error creating container")
	}

	c.containerID = container.ID

	if c.networkID != "" {
		err := c.cli.NetworkConnect(ctx, c.networkID, c.containerID, &network.EndpointSettings{
			Aliases: []string{c.name},
		})
		if err != nil {
			return err
		}
	}

	err = c.cli.ContainerStart(ctx, c.containerID, dockerContainer.StartOptions{})
	if err != nil {
		return errors.Wrap(err, "error starting container")
	}

	// Run in-container lifecycle commands (if any), in order:
	//  1. startup command — fail fast on exec error or non-zero exit.
	//  2. after-ready command — optionally gated on a readiness matcher
	//     (AwaitOutput), then fail fast on exec error or non-zero exit.
	if len(c.startupCmd) > 0 {
		if err := c.runLifecycleExec(ctx, c.startupCmd); err != nil {
			return err
		}
	}

	if len(c.afterReadyCmd) > 0 {
		if c.afterReadyMatcher != nil {
			if err := c.AwaitOutput(ctx, c.afterReadyMatcher); err != nil {
				return errors.Wrap(err, "error awaiting readiness for after-ready command")
			}
		}

		if err := c.runLifecycleExec(ctx, c.afterReadyCmd); err != nil {
			return err
		}
	}

	return nil
}

// runLifecycleExec runs a single lifecycle command inside the container and
// fails fast when the exec infrastructure fails or the command exits non-zero.
func (c *container) runLifecycleExec(ctx context.Context, cmd []string) error {
	execCtx, cancel := c.lifecycleExecContext(ctx)
	defer cancel()

	res, err := c.Exec(execCtx, cmd)
	if err != nil {
		return errors.Wrap(err, "error executing lifecycle command")
	}
	if res.ExitCode != 0 {
		return errors.Errorf("lifecycle command exited with code %d: %s",
			res.ExitCode, string(res.Stderr))
	}
	return nil
}

// lifecycleExecContext returns a context bounded by the caller's deadline when
// one is present, or by defaultExecTimeout when the caller provided none. This
// keeps individual lifecycle commands from hanging Run() indefinitely when no
// deadline is set, while still respecting an explicit caller deadline.
func (c *container) lifecycleExecContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, defaultExecTimeout)
}

// Close cleans up the env (stops & removes the container)
func (c *container) Close(ctx context.Context) error {
	if c.containerID == "" {
		return nil
	}

	// If the provided context is already expired, use a fresh one for cleanup
	// so a test timeout doesn't prevent container removal.
	if dl, ok := ctx.Deadline(); ok && time.Until(dl) <= 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), defaultStopTimeout)
		defer cancel()
	}

	timeout := defaultStopTimeout
	if dl, ok := ctx.Deadline(); ok {
		if remaining := time.Until(dl); remaining > 0 {
			// Cap the graceful-stop window at defaultStopTimeout instead of
			// inheriting the whole remaining context. Otherwise a slow-to-stop
			// container (e.g. an old OpenSearch) makes Docker wait out the
			// caller's context on the stop call and the cleanup itself fails
			// with a context deadline exceeded. Docker force-kills the
			// container once `timeout` elapses, so the stop stays bounded.
			if remaining < timeout {
				timeout = remaining
			}
		}
	}

	err := c.cli.ContainerStop(ctx, c.containerID, dockerContainer.StopOptions{
		Timeout: ptr.Ptr[int](int(timeout / time.Second)),
	})
	if err != nil {
		return err
	}

	err = c.cli.ContainerRemove(ctx, c.containerID, dockerContainer.RemoveOptions{
		RemoveVolumes: true,
		Force:         true,
	})
	if err != nil {
		return err
	}

	return nil
}

func (c *container) pullImage(ctx context.Context) error {
	if c.isUpToDate(ctx) {
		return nil
	}

	rc, err := c.cli.ImagePull(ctx, c.image, image.PullOptions{
		RegistryAuth: registryAuthForImage(c.image),
	})
	if err != nil {
		return errors.Wrap(err, "error pulling image")
	}
	defer func() { _ = rc.Close() }()

	// Drain the pull response to wait for the pull to complete.
	_, err = io.Copy(io.Discard, rc)
	if err != nil {
		return errors.Wrap(err, "error waiting for image pull to complete")
	}

	return nil
}

// isUpToDate reports whether the image does not need to be pulled.
//
// For non-":latest" images this means it is already present locally (see
// isImagePulled). For ":latest" images it is a best-effort comparison between
// the local manifest digest and the remote one; when the remote digest cannot
// be determined (registry unreachable, auth missing, no digest header) we
// conservatively report "not up to date" so the caller pulls as before.
func (c *container) isUpToDate(ctx context.Context) bool {
	if !strings.HasSuffix(c.image, ":latest") {
		return c.isImagePulled(ctx) == nil
	}

	local := localRepoDigest(ctx, c.cli, c.image)
	if local == "" {
		return false
	}

	remote, err := c.remoteManifestDigest(ctx)
	if err != nil {
		log.WithError(err).Tracef("could not determine remote digest for %s; falling back to pull", c.image)
		return false
	}

	if remote == "" {
		return false
	}

	if remote != local {
		log.WithFields(log.Fields{
			"image":  c.image,
			"local":  local,
			"remote": remote,
		}).Trace("image digest differs from remote; pulling")
		return false
	}

	log.WithFields(log.Fields{
		"image": c.image,
	}).Trace("image is up to date; skipping pull")
	return true
}

func (c *container) isImagePulled(ctx context.Context) error {
	images, err := c.cli.ImageList(ctx, image.ListOptions{})
	if err != nil {
		return err
	}

	// Compare normalized (familiar) image references instead of doing an
	// exact string match. Docker stores RepoTags in a canonical/short form
	// (e.g. "memcached:1.6.29" for "index.docker.io/library/memcached:1.6.29"),
	// so a raw string comparison would spuriously report the image as not
	// pulled and trigger a redundant pull on every Run().
	want := familiarImageRef(c.image)

	for _, image := range images {
		for _, tag := range image.RepoTags {
			if familiarImageRef(tag) == want {
				return nil
			}
		}
	}

	return errImageIsNotPulled
}

// familiarImageRef normalizes an image reference to its familiar (short)
// form, falling back to the raw string when it cannot be parsed (e.g.
// "<none>:<none>").
func familiarImageRef(ref string) string {
	named, err := reference.ParseNormalizedNamed(ref)
	if err != nil {
		return ref
	}
	return reference.FamiliarString(named)
}

// URL returns host & port pair to allow external connections
func (c *container) URL(proto Protocol, port uint16) (*HostPort, error) {
	log.WithFields(log.Fields{
		"proto":   proto.String(),
		"port":    port,
		"mapping": c.ports.portBindings,
	}).Trace("looking up for port ...")

	k := strconv.FormatUint(uint64(port), 10) + "/" + proto.String()
	if v, ok := c.ports.portAliases[k]; ok {
		k = v
	}

	pbs, ok := c.ports.portBindings[k]
	if !ok {
		return nil, errors.Errorf("port `%s` is not registered", k)
	}

	if len(pbs) != 1 {
		return nil, errors.New("unexpected amount of ports returned by name: mostly possible programmer error!")
	}

	dockerIP, err := DockerIP()
	if err != nil {
		return nil, err
	}

	hp := pbs[0].HostPort
	if hp == "" {
		return nil, errors.Errorf("external port is not defined for `%d`", port)
	}

	p, err := strconv.ParseUint(hp, 10, 16)
	if err != nil {
		return nil, err
	}

	return &HostPort{
		Host: dockerIP,
		Port: uint16(p),
	}, nil
}
