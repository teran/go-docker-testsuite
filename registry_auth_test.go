package docker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/cli/cli/config"
	"github.com/docker/docker/api/types/registry"
	"github.com/stretchr/testify/require"
)

func TestRegistryAuthForImage(t *testing.T) {
	r := require.New(t)

	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{
		"auths": {
			"registry.homelab.teran.dev": {
				"auth": "dXNlcjpwYXNz"
			}
		}
	}`), 0o600)
	r.NoError(err)

	// config.Dir() is cached once per process via sync.Once, so the DOCKER_CONFIG
	// env var may already be snapshotted by an earlier test. Use config.SetDir()
	// (as docker/cli's own tests do) so this test is hermetic regardless of order,
	// and restore the previous directory afterwards.
	prevDir := config.Dir()
	config.SetDir(dir)
	t.Cleanup(func() { config.SetDir(prevDir) })

	t.Run("auth present for matching domain", func(t *testing.T) {
		r := require.New(t)
		got := registryAuthForImage("registry.homelab.teran.dev/index.docker.io/library/postgres:17.5")
		r.NotEmpty(got)

		ac, err := registry.DecodeAuthConfig(got)
		r.NoError(err)
		r.Equal("user", ac.Username)
		r.Equal("pass", ac.Password)
		r.Equal("registry.homelab.teran.dev", ac.ServerAddress)
	})

	t.Run("no auth for public docker hub image", func(t *testing.T) {
		r := require.New(t)
		r.Equal("", registryAuthForImage("postgres:17.5"))
		r.Equal("", registryAuthForImage("docker.io/library/postgres:17.5"))
	})

	t.Run("no auth for unconfigured registry", func(t *testing.T) {
		r := require.New(t)
		r.Equal("", registryAuthForImage("quay.io/some/private/image:1.0"))
	})

	t.Run("no auth for unparseable image ref", func(t *testing.T) {
		r := require.New(t)
		r.Equal("", registryAuthForImage(":"))
	})
}
