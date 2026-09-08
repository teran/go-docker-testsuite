package libvirtd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/digitalocean/go-libvirt"
	"github.com/docker/docker/client"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"github.com/teran/go-docker-testsuite/images"
)

func init() {
	log.SetLevel(log.TraceLevel)
}

// requireDocker skips the test when running with -short or when the Docker
// daemon is not reachable, so this integration test degrades gracefully on
// machines without Docker instead of failing hard.
func requireDocker(t *testing.T) {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping libvirtd integration test in -short mode")
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		t.Skipf("skipping libvirtd test: unable to create docker client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := cli.Ping(ctx); err != nil {
		t.Skipf("skipping libvirtd test: docker daemon unavailable: %v", err)
	}
}

// listAllEntities exercises the "list everything" surface of the connected
// go-libvirt client: domains, networks, storage pools (and their volumes),
// secrets, nwfilters, node devices and interfaces. It fails if any listing
// call errors, proving the connection reaches all parts of libvirtd.
func listAllEntities(t *testing.T, conn *libvirt.Libvirt) {
	t.Helper()
	r := require.New(t)

	domains, _, err := conn.ConnectListAllDomains(1,
		libvirt.ConnectListDomainsActive|libvirt.ConnectListDomainsInactive)
	r.NoError(err)
	t.Logf("domains: %d", len(domains))

	networks, _, err := conn.ConnectListAllNetworks(1,
		libvirt.ConnectListNetworksActive|libvirt.ConnectListNetworksInactive)
	r.NoError(err)
	t.Logf("networks: %d", len(networks))

	pools, _, err := conn.ConnectListAllStoragePools(1,
		libvirt.ConnectListStoragePoolsActive|libvirt.ConnectListStoragePoolsInactive)
	r.NoError(err)
	t.Logf("storage pools: %d", len(pools))
	for _, p := range pools {
		vols, _, err := conn.StoragePoolListAllVolumes(p, 1, 0)
		r.NoError(err)
		t.Logf("  pool %q: %d volumes", p.Name, len(vols))
	}

	secrets, _, err := conn.ConnectListAllSecrets(1,
		libvirt.ConnectListSecretsEphemeral|libvirt.ConnectListSecretsNoPrivate)
	r.NoError(err)
	t.Logf("secrets: %d", len(secrets))

	nwfilters, _, err := conn.ConnectListAllNwfilters(1, 0)
	r.NoError(err)
	t.Logf("nwfilters: %d", len(nwfilters))

	devices, _, err := conn.ConnectListAllNodeDevices(1, 0)
	r.NoError(err)
	t.Logf("node devices: %d", len(devices))

	ifaces, _, err := conn.ConnectListAllInterfaces(1, 0)
	r.NoError(err)
	t.Logf("interfaces: %d", len(ifaces))
}

func TestLibvirtd(t *testing.T) {
	requireDocker(t)

	r := require.New(t)

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()

	app, err := NewWithImage(ctx, images.Libvirtd)
	r.NoError(err)
	r.NotNil(app)

	defer func() {
		cleanupCtx, cCancel := context.WithTimeout(context.Background(), time.Minute)
		defer cCancel()
		r.NoError(app.Close(cleanupCtx))
	}()

	// Addr must be a non-empty host:port for the libvirtd TCP endpoint.
	r.NotEmpty(app.Addr())

	// The client must be connected and report a non-zero libvirt version. This
	// does not hard-depend on /dev/kvm: without it QEMU falls back to software
	// (TCG) emulation, but libvirtd still serves its control socket.
	ver, err := app.Client().ConnectGetLibVersion()
	r.NoError(err)
	r.NotZero(ver)

	// Enumerate all libvirt entity types to prove the connection reaches every
	// part of the daemon.
	listAllEntities(t, app.Client())
}

// cirrosArch maps the host GOARCH to the libvirt arch name used in the cirros
// image filename and the domain <os><type arch> element.
func cirrosArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	default:
		return ""
	}
}

func cirrosURL(arch string) string {
	return "https://download.cirros-cloud.net/0.6.2/cirros-0.6.2-" + arch + "-disk.img"
}

// downloadFile fetches url into dst, honoring ctx cancellation.
func downloadFile(ctx context.Context, url, dst string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %s fetching %s", resp.Status, url)
	}

	//nolint:gosec // dst is a test temp-dir path
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = io.Copy(f, resp.Body)
	return err
}

// cirrosDomainXML builds a minimal domain definition that boots cirros from
// the given qcow2 disk path.
func cirrosDomainXML(name, diskPath, arch string) string {
	return fmt.Sprintf(`<domain type='qemu'>
  <name>%s</name>
  <memory unit='KiB'>262144</memory>
  <vcpu>1</vcpu>
  <os>
    <type arch='%s'>hvm</type>
    <boot dev='hd'/>
  </os>
  <devices>
    <disk type='file' device='disk'>
      <driver name='qemu' type='qcow2'/>
      <source file='%s'/>
      <target dev='vda' bus='virtio'/>
    </disk>
    <serial type='pty'/>
    <console type='pty'/>
  </devices>
</domain>`, name, arch, diskPath)
}

// TestLibvirtdCreateCirrosVM boots a real cirros cloud image inside the
// containerized libvirtd: it downloads the image, uploads it as a storage
// volume into a dir storage pool, defines and starts a domain, and verifies
// the domain reaches the running state.
func TestLibvirtdCreateCirrosVM(t *testing.T) {
	requireDocker(t)

	r := require.New(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	// Booting a real VM needs cgroup creation, which requires a privileged
	// container (least-privilege is enough for connect/list/storage but not for
	// launching QEMU). The Docker daemon is always Linux, so no host-OS gate is
	// needed — it works on any Docker with a proper cgroup v2 hierarchy.
	app, err := New(ctx, WithPrivileged())
	r.NoError(err)
	r.NotNil(app)
	defer func() {
		cleanupCtx, cCancel := context.WithTimeout(context.Background(), time.Minute)
		defer cCancel()
		r.NoError(app.Close(cleanupCtx))
	}()

	conn := app.Client()

	arch := cirrosArch()
	if arch == "" {
		t.Skipf("unsupported GOARCH %q for cirros image", runtime.GOARCH)
	}

	imgPath := filepath.Join(t.TempDir(), "cirros-disk.img")
	r.NoError(downloadFile(ctx, cirrosURL(arch), imgPath))
	t.Logf("downloaded cirros image to %s", imgPath)

	const (
		poolName   = "cirros-pool"
		volName    = "cirros.qcow2"
		domainName = "cirros"
	)

	// Create a directory storage pool inside the container.
	poolXML := fmt.Sprintf(
		`<pool type='dir'><name>%s</name><target><path>/var/lib/libvirt/images</path></target></pool>`,
		poolName)
	pool, err := conn.StoragePoolDefineXML(poolXML, 0)
	r.NoError(err)
	defer func() {
		_ = conn.StoragePoolDestroy(pool)
		_ = conn.StoragePoolUndefine(pool)
	}()
	r.NoError(conn.StoragePoolCreate(pool, 0))

	// Create a qcow2 volume and stream the cirros image into it.
	volXML := fmt.Sprintf(
		`<volume><name>%s</name><capacity unit='G'>1</capacity><allocation unit='G'>0</allocation><target><format type='qcow2'/></target></volume>`,
		volName)
	vol, err := conn.StorageVolCreateXML(pool, volXML, 0)
	r.NoError(err)
	defer func() { _ = conn.StorageVolDelete(vol, 0) }()

	//nolint:gosec // imgPath is a test temp-dir path
	f, err := os.Open(imgPath)
	r.NoError(err)
	defer func() { _ = f.Close() }()
	r.NoError(conn.StorageVolUpload(vol, f, 0, 0, 0))

	diskPath := "/var/lib/libvirt/images/" + volName

	// Define and start the domain.
	dom, err := conn.DomainDefineXML(cirrosDomainXML(domainName, diskPath, arch))
	r.NoError(err)
	defer func() {
		_ = conn.DomainDestroy(dom)
		_ = conn.DomainUndefine(dom)
	}()
	r.NoError(conn.DomainCreate(dom))

	// The domain must be running (VIR_DOMAIN_RUNNING == 1). This only requires
	// QEMU to start (TCG is fine without /dev/kvm), not a full guest boot.
	state, _, err := conn.DomainGetState(dom, 0)
	r.NoError(err)
	r.Equal(int32(1), state, "expected domain to be running")

	// It must show up in the domain listing.
	domains, _, err := conn.ConnectListAllDomains(1,
		libvirt.ConnectListDomainsActive|libvirt.ConnectListDomainsInactive)
	r.NoError(err)
	found := false
	for _, d := range domains {
		if d.Name == domainName {
			found = true
			break
		}
	}
	r.True(found, "cirros domain %q not found in domain listing", domainName)
}

// TestLibvirtdNetworkCRUD exercises a full create/read/delete round-trip on a
// libvirt virtual network, confirming that our container launch does not cut
// off libvirt's networking functionality. Starting a network bridge requires
// writing to /proc/sys (writable only in a privileged container).
func TestLibvirtdNetworkCRUD(t *testing.T) {
	requireDocker(t)

	r := require.New(t)

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()

	app, err := New(ctx, WithPrivileged())
	r.NoError(err)
	r.NotNil(app)
	defer func() {
		cleanupCtx, cCancel := context.WithTimeout(context.Background(), time.Minute)
		defer cCancel()
		r.NoError(app.Close(cleanupCtx))
	}()

	conn := app.Client()

	const netName = "crud-network"

	// Create: define + activate an isolated network.
	netXML := fmt.Sprintf(`<network><name>%s</name><bridge name='virbr-crud'/></network>`, netName)
	net, err := conn.NetworkDefineXML(netXML)
	r.NoError(err)
	defer func() { _ = conn.NetworkUndefine(net) }()
	r.NoError(conn.NetworkCreate(net))
	defer func() { _ = conn.NetworkDestroy(net) }()

	// Read: present in the listing and its XML/bridge are retrievable.
	nets, _, err := conn.ConnectListAllNetworks(1,
		libvirt.ConnectListNetworksActive|libvirt.ConnectListNetworksInactive)
	r.NoError(err)
	found := false
	for _, n := range nets {
		if n.Name == netName {
			found = true
			break
		}
	}
	r.True(found, "network %q not found in listing", netName)

	gotXML, err := conn.NetworkGetXMLDesc(net, 0)
	r.NoError(err)
	r.Contains(gotXML, netName)

	br, err := conn.NetworkGetBridgeName(net)
	r.NoError(err)
	r.NotEmpty(br)

	// Lookup by name resolves to the same network.
	looked, err := conn.NetworkLookupByName(netName)
	r.NoError(err)
	r.Equal(netName, looked.Name)

	// Delete: destroy + undefine, then confirm it is gone.
	r.NoError(conn.NetworkDestroy(net))
	r.NoError(conn.NetworkUndefine(net))

	_, err = conn.NetworkLookupByName(netName)
	r.Error(err, "expected network %q to be gone after undefine", netName)
}
