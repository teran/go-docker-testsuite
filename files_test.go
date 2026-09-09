package docker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Unit tests: FileFromBytes + WithFiles option factory
// ---------------------------------------------------------------------------

// TestFileFromBytes verifies that the FileFromBytes helper fills every field of
// a File correctly: Content is an in-memory reader over the exact data, Size
// equals len(data), and Mode/Uid/Gid/Destination are passed through verbatim.
func TestFileFromBytes(t *testing.T) {
	testCases := []struct {
		name string
		dest string
		data []byte
		mode os.FileMode
		uid  int
		gid  int
	}{
		{name: "empty content", dest: "/a", data: nil, mode: 0, uid: 0, gid: 0},
		{name: "text content", dest: "/etc/myapp/config.yaml", data: []byte("config-value\n"), mode: 0600, uid: 1000, gid: 1001},
		{name: "binary content", dest: "/data/blob", data: []byte{0x00, 0x01, 0x02, 0xff}, mode: 0644, uid: 1, gid: 2},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := require.New(t)

			f := FileFromBytes(tc.dest, tc.data, tc.mode, tc.uid, tc.gid)

			r.Equal(tc.dest, f.Destination)
			r.Equal(int64(len(tc.data)), f.Size)
			r.Equal(tc.mode, f.Mode)
			r.Equal(tc.uid, f.Uid)
			r.Equal(tc.gid, f.Gid)

			// Content must be an in-memory reader that yields exactly data.
			r.IsType(&bytes.Reader{}, f.Content)
			got, err := io.ReadAll(f.Content)
			r.NoError(err)
			r.True(bytes.Equal(tc.data, got), "content mismatch: got %v, want %v", got, tc.data)
		})
	}
}

// TestWithFiles verifies that the WithFiles option stores every file in
// c.files, preserving the streamed Content, Size, Mode, Destination and the
// numeric owner (Uid/Gid). Note that Content is an io.Reader whose state is
// advanced by reading, so each stored reader is consumed exactly once here.
func TestWithFiles(t *testing.T) {
	r := require.New(t)

	inputs := []File{
		FileFromBytes("/etc/a", []byte("a\n"), 0, 0, 0),
		{
			Content:     bytes.NewReader([]byte("b\n")),
			Size:        2,
			Mode:        0600,
			Destination: "/data/nested/b",
			Uid:         1000,
			Gid:         1001,
		},
	}
	wantData := [][]byte{
		[]byte("a\n"),
		[]byte("b\n"),
	}

	c := &container{}
	WithFiles(inputs...)(c)

	r.Len(c.files, len(inputs))
	for i, f := range c.files {
		r.Equal(inputs[i].Destination, f.Destination)
		r.Equal(inputs[i].Size, f.Size)
		r.Equal(inputs[i].Mode, f.Mode)
		r.Equal(inputs[i].Uid, f.Uid)
		r.Equal(inputs[i].Gid, f.Gid)

		got, err := io.ReadAll(f.Content)
		r.NoError(err)
		r.True(bytes.Equal(wantData[i], got), "content mismatch: got %v, want %v", got, wantData[i])
	}
}

// TestWithFilesEmpty verifies that an empty file list is a no-op: the option
// must not allocate or otherwise mark any files to be copied.
func TestWithFilesEmpty(t *testing.T) {
	r := require.New(t)

	c := &container{}
	WithFiles()(c)

	r.Empty(c.files)
	r.Nil(c.files)
}

// ---------------------------------------------------------------------------
// Integration tests (require Docker)
// ---------------------------------------------------------------------------

// TestContainerWithFiles verifies that files are present in the container
// after Run with the expected content, permission bits and numeric owner: a
// default-mode (0 → 0644) root:root file at the container root and an
// explicitly 0600 secret owned by 1000:1001 in a nested directory. It also
// confirms the auto-created parent directory stays root:root (0755).
func TestContainerWithFiles(t *testing.T) {
	r := require.New(t)
	requireDocker(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	c, err := NewContainerWithLifecycle(
		"files-test",
		busyboxImage,
		[]string{"sleep", "300"},
		NewEnvironment(),
		NewPortBindings(),
		WithFiles(
			FileFromBytes("/etc/myapp/config.yaml", []byte("config-value\n"), 0, 0, 0),
			FileFromBytes("/data/nested/secret.txt", []byte("s3cr3t\n"), 0600, 1000, 1001),
		),
	)
	r.NoError(err)
	defer func() { _ = c.Close(ctx) }()

	err = c.Run(ctx)
	r.NoError(err)

	t.Run("default mode file content", func(t *testing.T) {
		r := require.New(t)

		res, err := c.Exec(ctx, []string{"cat", "/etc/myapp/config.yaml"})
		r.NoError(err)
		r.Equal(0, res.ExitCode)
		r.Equal("config-value\n", string(res.Stdout))
		r.Empty(res.Stderr)
	})

	t.Run("default mode is 0644", func(t *testing.T) {
		r := require.New(t)

		res, err := c.Exec(ctx, []string{"stat", "-c", "%a", "/etc/myapp/config.yaml"})
		r.NoError(err)
		r.Equal(0, res.ExitCode)
		r.Equal("644", strings.TrimSpace(string(res.Stdout)))
	})

	t.Run("default owner is root:root", func(t *testing.T) {
		r := require.New(t)

		res, err := c.Exec(ctx, []string{"stat", "-c", "%u:%g", "/etc/myapp/config.yaml"})
		r.NoError(err)
		r.Equal(0, res.ExitCode)
		r.Equal("0:0", strings.TrimSpace(string(res.Stdout)))
	})

	t.Run("explicit 0600 secret content", func(t *testing.T) {
		r := require.New(t)

		res, err := c.Exec(ctx, []string{"cat", "/data/nested/secret.txt"})
		r.NoError(err)
		r.Equal(0, res.ExitCode)
		r.Equal("s3cr3t\n", string(res.Stdout))
	})

	t.Run("explicit mode is 0600", func(t *testing.T) {
		r := require.New(t)

		res, err := c.Exec(ctx, []string{"stat", "-c", "%a", "/data/nested/secret.txt"})
		r.NoError(err)
		r.Equal(0, res.ExitCode)
		r.Equal("600", strings.TrimSpace(string(res.Stdout)))
	})

	t.Run("explicit owner is uid:gid", func(t *testing.T) {
		r := require.New(t)

		res, err := c.Exec(ctx, []string{"stat", "-c", "%u:%g", "/data/nested/secret.txt"})
		r.NoError(err)
		r.Equal(0, res.ExitCode)
		r.Equal("1000:1001", strings.TrimSpace(string(res.Stdout)))
	})

	t.Run("parent directory stays root:root 0755", func(t *testing.T) {
		r := require.New(t)

		res, err := c.Exec(ctx, []string{"stat", "-c", "%u:%g:%a", "/data/nested"})
		r.NoError(err)
		r.Equal(0, res.ExitCode)
		r.Equal("0:0:755", strings.TrimSpace(string(res.Stdout)))
	})
}

// TestContainerWithFilesOwner verifies that the numeric owner (Uid/Gid) set on
// a File is applied to the copied file inside the container, and that the
// auto-created parent directory remains root:root (0755) even when the file
// itself carries a non-root owner.
func TestContainerWithFilesOwner(t *testing.T) {
	r := require.New(t)
	requireDocker(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	c, err := NewContainerWithLifecycle(
		"files-owner-test",
		busyboxImage,
		[]string{"sleep", "300"},
		NewEnvironment(),
		NewPortBindings(),
		WithFiles(
			FileFromBytes("/etc/app/owned.txt", []byte("owned\n"), 0600, 1000, 1001),
		),
	)
	r.NoError(err)
	defer func() { _ = c.Close(ctx) }()

	err = c.Run(ctx)
	r.NoError(err)

	t.Run("file owner is uid:gid", func(t *testing.T) {
		r := require.New(t)

		res, err := c.Exec(ctx, []string{"stat", "-c", "%u:%g", "/etc/app/owned.txt"})
		r.NoError(err)
		r.Equal(0, res.ExitCode)
		r.Equal("1000:1001", strings.TrimSpace(string(res.Stdout)))
	})

	t.Run("parent directory stays root:root", func(t *testing.T) {
		r := require.New(t)

		res, err := c.Exec(ctx, []string{"stat", "-c", "%u:%g:%a", "/etc/app"})
		r.NoError(err)
		r.Equal(0, res.ExitCode)
		r.Equal("0:0:755", strings.TrimSpace(string(res.Stdout)))
	})
}

// TestContainerWithFilesOwnerDefaultRoot verifies backward compatibility: a
// File that omits Uid/Gid is owned by root:root (0:0) inside the container,
// exactly as before the owner feature was introduced.
func TestContainerWithFilesOwnerDefaultRoot(t *testing.T) {
	r := require.New(t)
	requireDocker(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	c, err := NewContainerWithLifecycle(
		"files-owner-default-test",
		busyboxImage,
		[]string{"sleep", "300"},
		NewEnvironment(),
		NewPortBindings(),
		WithFiles(
			FileFromBytes("/etc/default-owned.txt", []byte("root-owned\n"), 0, 0, 0),
		),
	)
	r.NoError(err)
	defer func() { _ = c.Close(ctx) }()

	err = c.Run(ctx)
	r.NoError(err)

	res, err := c.Exec(ctx, []string{"stat", "-c", "%u:%g", "/etc/default-owned.txt"})
	r.NoError(err)
	r.Equal(0, res.ExitCode)
	r.Equal("0:0", strings.TrimSpace(string(res.Stdout)))
}

// TestContainerWithFilesFromOSFile verifies the streaming path: a file larger
// than any typical in-memory buffer (5 MiB) is streamed from an *os.File via
// File{Content: reader, Size: st.Size()} and copied byte-for-byte into the
// container. We confirm both the byte count (wc -c) and the exact content
// (sha256sum) match, proving no truncation or buffering loss.
func TestContainerWithFilesFromOSFile(t *testing.T) {
	r := require.New(t)
	requireDocker(t)

	const size = 5 * 1024 * 1024 // 5 MiB, well beyond any copy buffer

	// Deterministic pseudo-random data so the container-side hash can be
	// compared against a locally computed value.
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 251)
	}
	expectedSum := sha256.Sum256(data)

	// Write the source file into a temp dir and reopen it as a streamed reader.
	fpath := filepath.Join(t.TempDir(), "big.bin")
	r.NoError(os.WriteFile(fpath, data, 0600))
	st, err := os.Stat(fpath)
	r.NoError(err)

	//nolint:gosec // fpath is a trusted test temp-dir path
	f, err := os.Open(fpath)
	r.NoError(err)
	defer func() { _ = f.Close() }()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	c, err := NewContainerWithLifecycle(
		"files-osfile-test",
		busyboxImage,
		[]string{"sleep", "300"},
		NewEnvironment(),
		NewPortBindings(),
		WithFiles(
			File{
				Content:     f,
				Size:        st.Size(),
				Mode:        0644,
				Destination: "/data/big.bin",
			},
		),
	)
	r.NoError(err)
	defer func() { _ = c.Close(ctx) }()

	err = c.Run(ctx)
	r.NoError(err)

	t.Run("byte count matches", func(t *testing.T) {
		r := require.New(t)

		res, err := c.Exec(ctx, []string{"wc", "-c", "/data/big.bin"})
		r.NoError(err)
		r.Equal(0, res.ExitCode)
		fields := strings.Fields(string(res.Stdout))
		require.NotEmpty(t, fields)
		r.Equal(strconv.Itoa(size), fields[0])
	})

	t.Run("sha256 matches", func(t *testing.T) {
		r := require.New(t)

		res, err := c.Exec(ctx, []string{"sha256sum", "/data/big.bin"})
		r.NoError(err)
		r.Equal(0, res.ExitCode)
		fields := strings.Fields(string(res.Stdout))
		require.NotEmpty(t, fields)
		r.Equal(hex.EncodeToString(expectedSum[:]), fields[0])
	})
}

// TestContainerWithFilesVisibleToStartupCommand verifies the ordering guarantee
// that WithFiles copies files into the container before WithStartupCommand
// runs: the startup command reads a seeded file and writes its contents to
// /tmp/out, which we then inspect via Exec.
func TestContainerWithFilesVisibleToStartupCommand(t *testing.T) {
	r := require.New(t)
	requireDocker(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	c, err := NewContainerWithLifecycle(
		"files-startup-test",
		busyboxImage,
		[]string{"sleep", "300"},
		NewEnvironment(),
		NewPortBindings(),
		WithFiles(
			FileFromBytes("/tmp/flag", []byte("flag-value\n"), 0, 0, 0),
		),
		WithStartupCommand("sh", "-c", "cat /tmp/flag > /tmp/out"),
	)
	r.NoError(err)
	defer func() { _ = c.Close(ctx) }()

	err = c.Run(ctx)
	r.NoError(err)

	res, err := c.Exec(ctx, []string{"cat", "/tmp/out"})
	r.NoError(err)
	r.Equal(0, res.ExitCode)
	r.Equal("flag-value\n", string(res.Stdout))
}

// TestContainerWithFilesNestedDirectories verifies that parent directories of
// a deeply-nested destination are auto-created, so no prior setup is needed.
func TestContainerWithFilesNestedDirectories(t *testing.T) {
	r := require.New(t)
	requireDocker(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	c, err := NewContainerWithLifecycle(
		"files-nested-test",
		busyboxImage,
		[]string{"sleep", "300"},
		NewEnvironment(),
		NewPortBindings(),
		WithFiles(
			FileFromBytes("/a/b/c/file.txt", []byte("deep\n"), 0, 0, 0),
		),
	)
	r.NoError(err)
	defer func() { _ = c.Close(ctx) }()

	err = c.Run(ctx)
	r.NoError(err)

	res, err := c.Exec(ctx, []string{"cat", "/a/b/c/file.txt"})
	r.NoError(err)
	r.Equal(0, res.ExitCode)
	r.Equal("deep\n", string(res.Stdout))
}

// TestContainerWithFilesEmpty verifies that an empty WithFiles() list is a
// no-op at runtime: the container starts successfully without any copy step.
func TestContainerWithFilesEmpty(t *testing.T) {
	r := require.New(t)
	requireDocker(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	c, err := NewContainerWithLifecycle(
		"files-empty-test",
		busyboxImage,
		[]string{"sleep", "300"},
		NewEnvironment(),
		NewPortBindings(),
		WithFiles(),
	)
	r.NoError(err)
	defer func() { _ = c.Close(ctx) }()

	err = c.Run(ctx)
	r.NoError(err)
}

// TestContainerWithFilesInvalidDestination verifies that invalid destinations
// (relative, empty, or containing "..") and a negative Size cause Run to fail
// fast (fail-fast), consistent with the other lifecycle steps.
func TestContainerWithFilesInvalidDestination(t *testing.T) {
	testCases := []struct {
		name string
		file File
	}{
		{name: "relative path", file: FileFromBytes("etc/relative.conf", []byte("x\n"), 0, 0, 0)},
		{name: "empty destination", file: FileFromBytes("", []byte("x\n"), 0, 0, 0)},
		{name: "traversal with ..", file: FileFromBytes("/etc/../../escape", []byte("x\n"), 0, 0, 0)},
		{name: "negative size", file: File{Content: strings.NewReader("x\n"), Size: -1, Destination: "/etc/neg"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := require.New(t)
			requireDocker(t)

			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
			defer cancel()

			c, err := NewContainerWithLifecycle(
				"files-invalid-test",
				busyboxImage,
				[]string{"sleep", "300"},
				NewEnvironment(),
				NewPortBindings(),
				WithFiles(tc.file),
			)
			r.NoError(err)
			defer func() { _ = c.Close(ctx) }()

			err = c.Run(ctx)
			r.Error(err)
		})
	}
}
