package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/distribution/reference"
	"github.com/docker/cli/cli/config"
	"github.com/docker/docker/client"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// manifestAccept is the Accept header used to request (possibly multi-arch)
// manifests from a registry.
const manifestAccept = "application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.manifest.v1+json"

// registryHTTPClient is used for best-effort manifest lookups. A short
// timeout keeps a failing lookup cheap (the caller then falls back to pull).
var registryHTTPClient = &http.Client{Timeout: 8 * time.Second}

// localRepoDigest returns the local manifest digest for imageRef (from its
// RepoDigests), or "" when the image is not present locally.
func localRepoDigest(ctx context.Context, cli *client.Client, imageRef string) string {
	im, err := cli.ImageInspect(ctx, imageRef)
	if err != nil {
		return ""
	}

	for _, rd := range im.RepoDigests {
		if i := strings.LastIndexByte(rd, '@'); i >= 0 {
			return rd[i+1:]
		}
	}
	return ""
}

// remoteManifestDigest best-effort resolves the remote manifest digest of
// c.image directly from its registry (handling the anonymous/Docker-Hub token
// flow and registry-credential auth). It returns an error when the digest
// cannot be determined so the caller falls back to a normal pull.
func (c *container) remoteManifestDigest(ctx context.Context) (string, error) {
	named, err := reference.ParseNormalizedNamed(c.image)
	if err != nil {
		return "", err
	}

	domain := reference.Domain(named)
	path := reference.Path(named)
	tag := "latest"
	if t, ok := named.(reference.Tagged); ok {
		tag = t.Tag()
	}
	// Docker Hub serves manifests from a dedicated registry endpoint.
	if domain == "docker.io" || domain == "index.docker.io" {
		domain = "registry-1.docker.io"
	}

	username, password, identityToken := registryCredentials(domain)

	for _, scheme := range []string{"https", "http"} {
		manifestURL := fmt.Sprintf("%s://%s/v2/%s/manifests/%s", scheme, domain, path, tag)

		if d, err := fetchManifestDigest(ctx, manifestURL, "", ""); err == nil && d != "" {
			return d, nil
		}

		token, err := registryToken(ctx, manifestURL, username, password, identityToken)
		if err != nil || token == "" {
			continue
		}

		if d, err := fetchManifestDigest(ctx, manifestURL, "Bearer", token); err == nil && d != "" {
			return d, nil
		}
	}

	return "", errors.Errorf("could not determine remote manifest digest for %q", c.image)
}

// fetchManifestDigest performs a GET on url and returns the
// Docker-Content-Digest response header.
func fetchManifestDigest(ctx context.Context, url, authScheme, token string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", manifestAccept)
	if token != "" {
		req.Header.Set("Authorization", authScheme+" "+token)
	}

	resp, err := registryHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	return resp.Header.Get("Docker-Content-Digest"), nil
}

// registryToken resolves a bearer token for a registry using the standard
// WWW-Authenticate challenge flow, using the provided credentials (possibly
// empty for anonymous public access).
func registryToken(ctx context.Context, manifestURL, username, password, identityToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", manifestAccept)

	resp, err := registryHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusUnauthorized {
		return "", errors.New("no auth challenge")
	}

	challenge := resp.Header.Get("WWW-Authenticate")
	if !strings.HasPrefix(challenge, "Bearer") {
		return "", errors.New("unsupported auth scheme")
	}

	params := parseAuthParams(strings.TrimPrefix(challenge, "Bearer"))
	realm := params["realm"]
	if realm == "" {
		return "", errors.New("auth challenge has no realm")
	}

	tokenURL := realm
	if svc := params["service"]; svc != "" {
		tokenURL += "?service=" + url.QueryEscape(svc)
	}
	if scope := params["scope"]; scope != "" {
		sep := "?"
		if strings.Contains(tokenURL, "?") {
			sep = "&"
		}
		tokenURL += sep + "scope=" + url.QueryEscape(scope)
	}

	treq, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL, nil)
	if err != nil {
		return "", err
	}
	switch {
	case identityToken != "":
		treq.Header.Set("Authorization", "Bearer "+identityToken)
	case username != "":
		treq.SetBasicAuth(username, password)
	}

	tresp, err := registryHTTPClient.Do(treq)
	if err != nil {
		return "", err
	}
	defer func() { _ = tresp.Body.Close() }()

	var tr struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(tresp.Body).Decode(&tr); err != nil {
		return "", err
	}
	if tr.Token != "" {
		return tr.Token, nil
	}
	if tr.AccessToken != "" {
		return tr.AccessToken, nil
	}
	return "", errors.New("no token in registry response")
}

// registryCredentials returns the configured credentials for a registry
// domain from the user's Docker config file.
func registryCredentials(domain string) (username, password, identityToken string) {
	cf := config.LoadDefaultConfigFile(io.Discard)
	ac, err := cf.GetAuthConfig(domain)
	if err != nil {
		log.WithError(err).Tracef("no credentials for registry %s", domain)
		return "", "", ""
	}
	return ac.Username, ac.Password, ac.IdentityToken
}

// parseAuthParams parses the comma-separated key=value params of a
// WWW-Authenticate header value, respecting double-quoted values.
func parseAuthParams(s string) map[string]string {
	m := map[string]string{}
	for _, part := range splitOutsideQuotes(s, ',') {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		m[strings.TrimSpace(kv[0])] = strings.Trim(strings.TrimSpace(kv[1]), `"`)
	}
	return m
}

// splitOutsideQuotes splits s by sep, ignoring separators inside double quotes.
func splitOutsideQuotes(s string, sep rune) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
			cur.WriteRune(r)
		case r == sep && !inQuote:
			out = append(out, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}
