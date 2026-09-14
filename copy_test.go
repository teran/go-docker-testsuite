package docker

import (
	"archive/tar"
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Unit tests: CopyFromContainer package helper + capability resolution
// ---------------------------------------------------------------------------

// stubContainer is a minimal Container that does NOT implement the optional
// CopyFromContainer capability, so docker.CopyFromContainer must reject it.
type stubContainer struct{}

func (stubContainer) AwaitOutput(context.Context, Matcher) error          { return nil }
func (stubContainer) Close(context.Context) error                         { return nil }
func (stubContainer) Exec(context.Context, []string) (*ExecResult, error) { return nil, nil }
func (stubContainer) GetOutput(context.Context, ...Matcher) ([]string, error) {
	return nil, nil
}
func (stubContainer) ID() ContainerID                         { return "" }
func (stubContainer) Name() string                            { return "stub" }
func (stubContainer) NetworkAttach(string) error              { return nil }
func (stubContainer) Ping(context.Context) error              { return nil }
func (stubContainer) Run(context.Context) error               { return nil }
func (stubContainer) URL(Protocol, uint16) (*HostPort, error) { return nil, nil }

// stubCopyContainer embeds stubContainer and adds CopyFromContainer, so it
// satisfies both Container and ContainerFileCopier. Its CopyFromContainer
// succeeds, returning a nil reader to keep the unit test light.
type stubCopyContainer struct {
	stubContainer
}

func (stubCopyContainer) CopyFromContainer(context.Context, string) (io.ReadCloser, error) {
	return nil, nil
}

// TestCopyFromContainerUnsupported verifies that docker.CopyFromContainer
// returns an error (not a panic) when handed a Container that lacks the
// optional CopyFromContainer capability.
func TestCopyFromContainerUnsupported(t *testing.T) {
	r := require.New(t)

	rc, err := CopyFromContainer(context.Background(), stubContainer{}, "/etc/foo")
	r.Error(err)
	r.Nil(rc)
}

// TestCopyFromContainerSupported verifies that docker.CopyFromContainer
// resolves and delegates to a Container that implements the optional
// CopyFromContainer capability.
func TestCopyFromContainerSupported(t *testing.T) {
	r := require.New(t)

	rc, err := CopyFromContainer(context.Background(), stubCopyContainer{}, "/etc/foo")
	r.NoError(err)
	r.Nil(rc) // the stub returns a nil reader; we only assert delegation happened
}

// TestContainerFileCopierInterface verifies that TestContainer advertises the
// optional capability (it resolves it against its wrapped container), even
// though the underlying container field holds only the Container interface.
func TestContainerFileCopierInterface(t *testing.T) {
	r := require.New(t)

	tc := &TestContainer{c: stubCopyContainer{}}
	_, ok := interface{}(tc).(ContainerFileCopier)
	r.True(ok, "TestContainer should implement ContainerFileCopier")

	rc, err := tc.CopyFromContainer(context.Background(), "/etc/foo")
	r.NoError(err)
	r.Nil(rc)
}

// ---------------------------------------------------------------------------
// Integration tests (require Docker)
// ---------------------------------------------------------------------------

// TestContainerCopyFromContainerFile verifies that a file written inside a
// running container can be copied out and unpacked from the returned tar
// stream, with its content intact.
func TestContainerCopyFromContainerFile(t *testing.T) {
	r := require.New(t)
	requireDocker(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	c, err := NewContainer(
		"copy-file-test",
		busyboxImage,
		[]string{"sleep", "300"},
		NewEnvironment(),
		NewPortBindings(),
	)
	r.NoError(err)
	defer func() { _ = c.Close(ctx) }()

	r.NoError(c.Run(ctx))

	res, err := c.Exec(ctx, []string{"sh", "-c", "mkdir -p /data && echo 'hello-from-container' > /data/out.txt"})
	r.NoError(err)
	r.Equal(0, res.ExitCode)

	// Copy the file out via the package helper (c is the Container interface).
	rc, err := CopyFromContainer(ctx, c, "/data/out.txt")
	r.NoError(err)
	defer func() { _ = rc.Close() }()

	tr := tar.NewReader(rc)
	hdr, err := tr.Next()
	r.NoError(err)
	r.NotEmpty(hdr.Name)

	content, err := io.ReadAll(tr)
	r.NoError(err)
	r.Equal("hello-from-container\n", string(content))

	// Only the single requested file should be present.
	_, err = tr.Next()
	r.ErrorIs(err, io.EOF)
}

// TestContainerCopyFromContainerDirectory verifies copying a whole directory
// out of the container: every entry is present in the returned tar.
func TestContainerCopyFromContainerDirectory(t *testing.T) {
	r := require.New(t)
	requireDocker(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	c, err := NewContainer(
		"copy-dir-test",
		busyboxImage,
		[]string{"sleep", "300"},
		NewEnvironment(),
		NewPortBindings(),
	)
	r.NoError(err)
	defer func() { _ = c.Close(ctx) }()

	r.NoError(c.Run(ctx))

	res, err := c.Exec(ctx, []string{"sh", "-c", "mkdir -p /data && echo a > /data/a.txt && echo b > /data/b.txt"})
	r.NoError(err)
	r.Equal(0, res.ExitCode)

	rc, err := CopyFromContainer(ctx, c, "/data")
	r.NoError(err)
	defer func() { _ = rc.Close() }()

	contents := map[string]string{}
	tr := tar.NewReader(rc)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		r.NoError(err)
		if hdr.Typeflag == tar.TypeDir {
			continue
		}
		b, err := io.ReadAll(tr)
		r.NoError(err)
		contents[hdr.Name] = string(b)
	}

	// Docker names entries in the tar relative to the archive root, so a copy
	// of /data yields entries prefixed with "data/".
	r.Contains(contents, "data/a.txt")
	r.Contains(contents, "data/b.txt")
	r.Equal("a\n", contents["data/a.txt"])
	r.Equal("b\n", contents["data/b.txt"])
}

// TestContainerCopyFromContainerViaTestContainer verifies that the forwarding
// path through TestContainer (the *testing.T-bound decorator) copies files out
// of the wrapped container correctly.
func TestContainerCopyFromContainerViaTestContainer(t *testing.T) {
	r := require.New(t)
	requireDocker(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	tc, err := NewContainerWithT(
		t,
		"copy-via-testcontainer",
		busyboxImage,
		[]string{"sleep", "300"},
		NewEnvironment(),
		NewPortBindings(),
	)
	r.NoError(err)
	tc.RunT(ctx)

	res, err := tc.Exec(ctx, []string{"sh", "-c", "echo 'via-tc' > /tmp/out.txt"})
	r.NoError(err)
	r.Equal(0, res.ExitCode)

	rc, err := tc.CopyFromContainer(ctx, "/tmp/out.txt")
	r.NoError(err)
	defer func() { _ = rc.Close() }()

	tr := tar.NewReader(rc)
	_, err = tr.Next()
	r.NoError(err)

	content, err := io.ReadAll(tr)
	r.NoError(err)
	r.Equal("via-tc\n", string(content))
}
