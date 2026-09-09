package docker

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Unit tests: WithFiles option factory
// ---------------------------------------------------------------------------

func TestWithFiles(t *testing.T) {
	r := require.New(t)

	c := &container{}
	WithFiles(
		File{Content: []byte("a\n"), Destination: "/etc/a"},
		File{Content: []byte("b\n"), Mode: 0600, Destination: "/data/nested/b"},
	)(c)

	r.Equal([]File{
		{Content: []byte("a\n"), Destination: "/etc/a"},
		{Content: []byte("b\n"), Mode: 0600, Destination: "/data/nested/b"},
	}, c.files)
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
// after Run with the expected content and permission bits: a default-mode file
// (0 → 0644) at the container root and an explicitly 0600 secret in a nested
// directory.
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
			File{
				Content:     []byte("config-value\n"),
				Mode:        0, // default → 0644
				Destination: "/etc/myapp/config.yaml",
			},
			File{
				Content:     []byte("s3cr3t\n"),
				Mode:        0600,
				Destination: "/data/nested/secret.txt",
			},
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
			File{
				Content:     []byte("flag-value\n"),
				Destination: "/tmp/flag",
			},
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
			File{
				Content:     []byte("deep\n"),
				Destination: "/a/b/c/file.txt",
			},
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
// (relative, empty, or containing "..") cause Run to fail fast (fail-fast),
// consistent with the other lifecycle steps.
func TestContainerWithFilesInvalidDestination(t *testing.T) {
	testCases := []struct {
		name string
		dest string
	}{
		{name: "relative path", dest: "etc/relative.conf"},
		{name: "empty destination", dest: ""},
		{name: "traversal with ..", dest: "/etc/../../escape"},
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
				WithFiles(
					File{
						Content:     []byte("x\n"),
						Destination: tc.dest,
					},
				),
			)
			r.NoError(err)
			defer func() { _ = c.Close(ctx) }()

			err = c.Run(ctx)
			r.Error(err)
		})
	}
}
