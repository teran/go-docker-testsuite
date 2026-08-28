package docker

import (
	"io"

	"github.com/distribution/reference"
	"github.com/docker/cli/cli/config"
	"github.com/docker/cli/cli/config/types"
	"github.com/docker/docker/api/types/registry"
)

// registryAuthForImage returns the base64url-encoded registry auth header
// (X-Registry-Auth) for the registry hosting the given image reference, read
// from the user's Docker config file (~/.docker/config.json, honouring the
// DOCKER_CONFIG env var, credsStore and credHelpers).
//
// It returns an empty string when no credentials are configured for the image's
// registry, when the config file cannot be read/resolved, or when the image
// reference cannot be parsed. In all those cases the pull proceeds without auth,
// matching the public-registry behaviour.
func registryAuthForImage(imageRef string) string {
	named, err := reference.ParseNormalizedNamed(imageRef)
	if err != nil {
		// Unparseable image reference: fall back to pulling without auth.
		return ""
	}

	cf := config.LoadDefaultConfigFile(io.Discard)
	ac, err := cf.GetAuthConfig(reference.Domain(named))
	if err != nil {
		// Credentials could not be resolved (e.g. a failing creds helper): no auth.
		return ""
	}

	if !authCredentialsConfigured(ac) {
		// No credentials configured for this registry: no auth.
		return ""
	}

	enc, err := registry.EncodeAuthConfig(toRegistryAuthConfig(ac))
	if err != nil {
		return ""
	}

	return enc
}

// authCredentialsConfigured reports whether the resolved auth config actually
// carries credentials (as opposed to an empty "no credentials" result).
func authCredentialsConfigured(ac types.AuthConfig) bool {
	return ac.Username != "" || ac.IdentityToken != "" || ac.RegistryToken != ""
}

// toRegistryAuthConfig converts a docker/cli auth config into the docker/docker
// registry auth config type used by EncodeAuthConfig.
func toRegistryAuthConfig(ac types.AuthConfig) registry.AuthConfig {
	return registry.AuthConfig{
		Username:      ac.Username,
		Password:      ac.Password,
		Auth:          ac.Auth,
		ServerAddress: ac.ServerAddress,
		IdentityToken: ac.IdentityToken,
		RegistryToken: ac.RegistryToken,
	}
}
