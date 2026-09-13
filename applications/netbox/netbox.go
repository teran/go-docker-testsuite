// Package netbox provides a NetBox instance for integration testing.
//
// NetBox cannot run on SQLite — it requires an external PostgreSQL database and
// a Redis cache — so this wrapper runs a three-container docker.Group on an
// internal network where the siblings resolve each other by container name:
//
//	netbox-postgres (PostgreSQL, internal only)
//	netbox-redis    (Redis, internal only)
//	netbox          (the NetBox web app, port 8080 DNAT'd to the host)
//
// The caller brings their own HTTP client; no NetBox SDK dependency is embedded
// here (a deliberate decision to keep this stdlib-only).
package netbox

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	docker "github.com/teran/go-docker-testsuite"
	"github.com/teran/go-docker-testsuite/images"
)

const (
	// defaultSecretKey is a fixed, deterministic value used only for local
	// integration testing. It is intentionally insecure — never use it for
	// anything beyond spinning up a disposable test instance. Override it with
	// WithSecretKey when a real secret is needed.
	// #nosec G101 -- intentionally-insecure fixed test secret, documented above.
	defaultSecretKey = "go-docker-testsuite-netbox-insecure-test-only-secret"

	// defaultDBPassword is a fixed test-only password used for both the backing
	// PostgreSQL superuser and the NetBox application user. Override with
	// WithDBPassword.
	defaultDBPassword = "netbox-test-password"

	// Default superuser credentials. They are deterministic so tests can rely
	// on them without extra configuration. NetBox does not reserve the "admin"
	// username (unlike Forgejo), so "admin" is safe here.
	defaultSuperuserUsername = "admin"
	defaultSuperuserPassword = "NetBoxTest123!"
	defaultSuperuserEmail    = "admin@example.com"

	// defaultSuperuserAPIToken is the plaintext of the NetBox v2 API token
	// created for the superuser. NetBox 4.6 (netbox-docker 5.x) only issues v2
	// tokens; the full working credential is "nbt_<key>.<plaintext>" and is
	// authenticated with "Authorization: Bearer <credential>".
	defaultSuperuserAPIToken = "0123456789abcdef0123456789abcdef01234567"

	// defaultSuperuserAPIKey is the 12-character identification key of the v2
	// API token (SUPERUSER_API_KEY). It is a wrapper-internal constant: the
	// consumer never needs it directly, because SuperuserAPIToken() returns the
	// full "nbt_<key>.<plaintext>" credential.
	defaultSuperuserAPIKey = "abcdefghijkl"

	// defaultAPITokenPepper is the API_TOKEN_PEPPER_1 used to seed v2 token
	// digests. It is deterministic and intentionally insecure for testing; it
	// only gates token creation (the value is never exposed). NetBox requires
	// peppers to be at least 50 characters long.
	// #nosec G101 -- intentionally-insecure fixed test pepper, documented above.
	defaultAPITokenPepper = "go-docker-testsuite-netbox-api-token-pepper-0123456789abcdef"

	// Container names on the internal network. The NetBox container reaches its
	// dependencies by these DNS names.
	containerPostgres = "netbox-postgres"
	containerRedis    = "netbox-redis"
	containerNetBox   = "netbox"

	dbName    = "netbox"
	dbUser    = "netbox"
	dbPort    = "5432"
	redisPort = "6379"

	webPort = 8080
)

// NetBox exposes the minimal information a caller needs to talk to a running
// NetBox instance: its web URL and the superuser credentials created during
// setup. The caller brings their own HTTP/API client — no NetBox SDK dependency
// is embedded here (a deliberate decision to keep this stdlib-only).
type NetBox interface {
	URL() (string, error)
	MustURL() string
	SuperuserUsername() string
	SuperuserPassword() string
	// SuperuserAPIToken returns the full NetBox v2 API credential
	// ("nbt_<key>.<plaintext>") for use as "Authorization: Bearer <token>".
	SuperuserAPIToken() string
	Close(ctx context.Context) error
}

type netbox struct {
	g   docker.Group
	c   docker.Container
	cfg config
}

// superuserConfig carries the credentials of the NetBox superuser that is
// created on first startup.
type superuserConfig struct {
	username string
	password string
	apiToken string
	email    string
}

// config holds the consumer-tunable options for a NetBox instance.
type config struct {
	secretKey    string
	allowedHosts []string
	dbPassword   string
	superuser    *superuserConfig
}

// Option configures a NetBox instance.
type Option func(*config)

// WithSecretKey sets the Django SECRET_KEY. Defaults to a fixed, deterministic
// and deliberately insecure test-only value — override for anything that must
// not use a known secret.
func WithSecretKey(secretKey string) Option {
	return func(c *config) {
		c.secretKey = secretKey
	}
}

// WithAllowedHosts sets ALLOWED_HOSTS (space-joined into a single env var).
// Defaults to "*" so the instance answers on any host.
func WithAllowedHosts(hosts ...string) Option {
	return func(c *config) {
		c.allowedHosts = hosts
	}
}

// WithDBPassword sets the password used for the backing PostgreSQL instance
// (POSTGRES_PASSWORD / DB_PASSWORD). Defaults to a fixed test-only constant.
func WithDBPassword(password string) Option {
	return func(c *config) {
		c.dbPassword = password
	}
}

// WithSuperuser overrides the superuser created during setup (SUPERUSER_NAME /
// SUPERUSER_PASSWORD / SUPERUSER_API_TOKEN / SUPERUSER_EMAIL). apiToken is the
// plaintext of the NetBox v2 API token; SuperuserAPIToken() returns the full
// "nbt_<key>.<plaintext>" credential built from it. Defaults to a deterministic
// test account ("admin").
func WithSuperuser(username, password, apiToken, email string) Option {
	return func(c *config) {
		c.superuser = &superuserConfig{
			username: username,
			password: password,
			apiToken: apiToken,
			email:    email,
		}
	}
}

func defaultConfig() config {
	return config{
		secretKey:    defaultSecretKey,
		allowedHosts: []string{"*"},
		dbPassword:   defaultDBPassword,
		superuser: &superuserConfig{
			username: defaultSuperuserUsername,
			password: defaultSuperuserPassword,
			apiToken: defaultSuperuserAPIToken,
			email:    defaultSuperuserEmail,
		},
	}
}

// env returns the environment for the NetBox container.
//
// REDIS_CACHE_* is deliberately left unset: NetBox then defaults the cache to
// REDIS_HOST/REDIS_PORT, so a single Redis serves both the task queue (db0) and
// the cache (db1).
func env(cfg config) docker.Environment {
	e := docker.NewEnvironment().
		StringVar("DB_NAME", dbName).
		StringVar("DB_USER", dbUser).
		StringVar("DB_PASSWORD", cfg.dbPassword).
		StringVar("DB_HOST", containerPostgres).
		StringVar("DB_PORT", dbPort).
		StringVar("DB_SSLMODE", "disable").
		StringVar("REDIS_HOST", containerRedis).
		StringVar("REDIS_PORT", redisPort).
		StringVar("SECRET_KEY", cfg.secretKey).
		StringVar("ALLOWED_HOSTS", strings.Join(cfg.allowedHosts, " "))

	if cfg.superuser != nil {
		e = e.
			StringVar("SUPERUSER_NAME", cfg.superuser.username).
			StringVar("SUPERUSER_PASSWORD", cfg.superuser.password).
			StringVar("SUPERUSER_API_TOKEN", cfg.superuser.apiToken).
			StringVar("SUPERUSER_API_KEY", defaultSuperuserAPIKey).
			StringVar("SUPERUSER_EMAIL", cfg.superuser.email).
			// API_TOKEN_PEPPER_1 must be set or netbox-docker skips token
			// creation entirely.
			StringVar("API_TOKEN_PEPPER_1", defaultAPITokenPepper)
	}

	return e
}

// postgresReady is an AfterRun hook that blocks until PostgreSQL accepts
// connections, so the NetBox container (started later in the group) finds a
// live database.
func postgresReady(ctx context.Context, ht docker.HookType, c docker.Container) error {
	if ht == docker.HookTypeAfterRun {
		return c.AwaitOutput(ctx, docker.NewSubstringMatcher("database system is ready to accept connections"))
	}
	return nil
}

// redisReady is an AfterRun hook that blocks until Redis accepts connections.
func redisReady(ctx context.Context, ht docker.HookType, c docker.Container) error {
	if ht == docker.HookTypeAfterRun {
		return c.AwaitOutput(ctx, docker.NewSubstringMatcher("* Ready to accept connections"))
	}
	return nil
}

// New starts a NetBox instance backed by its own PostgreSQL and Redis
// containers on a shared internal network.
func New(ctx context.Context, image string, opts ...Option) (NetBox, error) {
	return newNetBox(ctx, nil, image, opts...)
}

// NewWithT is New bound to a *testing.T: the containers' lifecycles are tied to
// the test and cleaned up automatically via t.Cleanup.
func NewWithT(t *testing.T, ctx context.Context, image string, opts ...Option) (NetBox, error) {
	return newNetBox(ctx, t, image, opts...)
}

func newNetBox(ctx context.Context, t *testing.T, image string, opts ...Option) (NetBox, error) {
	cfg := defaultConfig()
	for _, o := range opts {
		o(&cfg)
	}

	pgEnv := docker.NewEnvironment().
		StringVar("POSTGRES_DB", dbName).
		StringVar("POSTGRES_USER", dbUser).
		StringVar("POSTGRES_PASSWORD", cfg.dbPassword)

	redisEnv := docker.NewEnvironment()

	nbEnv := env(cfg)

	var (
		pgC    docker.Container
		redisC docker.Container
		nbC    docker.Container
		err    error
	)

	if t != nil {
		pgC, err = docker.NewContainerWithT(t, containerPostgres, images.NetBoxPostgres, nil, pgEnv, docker.NewPortBindings())
		if err != nil {
			return nil, err
		}
		redisC, err = docker.NewContainerWithT(t, containerRedis, images.NetBoxRedis, nil, redisEnv, docker.NewPortBindings())
		if err != nil {
			return nil, err
		}
		nbC, err = docker.NewContainerWithT(t, containerNetBox, image, nil, nbEnv, docker.NewPortBindings().PortDNAT(docker.ProtoTCP, webPort))
		if err != nil {
			return nil, err
		}
	} else {
		pgC, err = docker.NewContainer(containerPostgres, images.NetBoxPostgres, nil, pgEnv, docker.NewPortBindings())
		if err != nil {
			return nil, err
		}
		redisC, err = docker.NewContainer(containerRedis, images.NetBoxRedis, nil, redisEnv, docker.NewPortBindings())
		if err != nil {
			return nil, err
		}
		nbC, err = docker.NewContainer(containerNetBox, image, nil, nbEnv, docker.NewPortBindings().PortDNAT(docker.ProtoTCP, webPort))
		if err != nil {
			return nil, err
		}
	}

	g, err := docker.NewGroup("netbox",
		docker.NewApplication(pgC, postgresReady),
		docker.NewApplication(redisC, redisReady),
		docker.NewApplication(nbC),
	)
	if err != nil {
		return nil, err
	}

	started := false
	defer func() {
		if !started {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_ = g.Close(cleanupCtx)
		}
	}()

	if err := g.Run(ctx); err != nil {
		return nil, err
	}

	n := &netbox{g: g, c: nbC, cfg: cfg}

	if err := n.awaitReady(ctx); err != nil {
		return nil, err
	}

	started = true
	return n, nil
}

// awaitReady implements the two-phase readiness check for the NetBox container.
//
// Phase A waits for the web server to print its "Listening at:" line — Granian
// emits this only after the database migrations have completed and the app has
// bound its socket. netbox-docker 5.x uses Granian (not gunicorn), so the
// gunicorn "Booting worker with pid" line never appears; we still accept it to
// stay robust against image variants that use gunicorn.
//
// Phase B (belt-and-braces) probes the HTTP API and verifies the superuser API
// token, guaranteeing the credentials exposed by the wrapper are usable (the
// netbox entrypoint does NOT fail if SUPERUSER_* creation fails, so the wrapper
// verifies it explicitly).
//
// The whole check is bounded by a 10-minute timeout; on expiry the wrapped
// error reports that NetBox never became ready. This is deliberately generous:
// the first start runs ~200 database migrations against a fresh PostgreSQL,
// which can take many minutes on hosts with slow disk I/O (e.g. Docker Desktop
// on macOS, where Postgres checkpoints have been observed taking ~100s).
func (n *netbox) awaitReady(ctx context.Context) error {
	readyCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	// Accept both the Granian "Listening at:" line (netbox-docker 5.x) and the
	// gunicorn "Booting worker with pid" line (older image variants).
	phaseA := docker.Matcher(func(l string) bool {
		return strings.Contains(l, "Listening at:") || strings.Contains(l, "Booting worker with pid")
	})

	if err := n.c.AwaitOutput(readyCtx, phaseA); err != nil {
		return errors.Wrap(err, "netbox did not become ready")
	}

	u, err := n.c.URL(docker.ProtoTCP, webPort)
	if err != nil {
		return errors.Wrap(err, "netbox did not become ready")
	}

	if err := n.probeAPI(readyCtx, fmt.Sprintf("http://%s", u.String())); err != nil {
		return errors.Wrap(err, "netbox did not become ready")
	}

	return nil
}

// probeAPI polls the NetBox API root until it answers and then verifies the
// superuser API token is accepted (a request carrying it must return 200).
func (n *netbox) probeAPI(ctx context.Context, baseURL string) error {
	client := &http.Client{Timeout: 5 * time.Second}

	// Phase B1: wait for the API to answer an unauthenticated request. NetBox
	// responds 403 to anonymous callers once it is serving, so any HTTP status
	// code indicates the API is reachable; the authoritative check is Phase B2.
	deadline := time.Now().Add(30 * time.Second)
	ok := false
	for !ok {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/", nil)
		if err != nil {
			return err
		}

		resp, err := client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			ok = true
			break
		}

		if time.Now().After(deadline) {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	if !ok {
		return errors.New("API did not answer within the timeout")
	}

	// Phase B2: the superuser API token must be accepted. NetBox 4.6 uses v2
	// tokens, so the credential is authenticated via "Bearer <credential>"
	// rather than "Token <token>".
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+n.SuperuserAPIToken())

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("superuser API token was rejected: HTTP %d", resp.StatusCode)
	}

	return nil
}

func (n *netbox) URL() (string, error) {
	u, err := n.c.URL(docker.ProtoTCP, webPort)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("http://%s", u.String()), nil
}

func (n *netbox) MustURL() string {
	u, err := n.URL()
	if err != nil {
		panic(err)
	}
	return u
}

func (n *netbox) SuperuserUsername() string {
	return n.cfg.superuser.username
}

func (n *netbox) SuperuserPassword() string {
	return n.cfg.superuser.password
}

// SuperuserAPIToken returns the full NetBox v2 API credential in the form
// "nbt_<key>.<plaintext>". Use it as "Authorization: Bearer <token>" — NetBox
// 4.6 (netbox-docker 5.x) authenticates API tokens with the "Bearer" scheme,
// not the legacy "Token" scheme used by earlier NetBox releases.
func (n *netbox) SuperuserAPIToken() string {
	return "nbt_" + defaultSuperuserAPIKey + "." + n.cfg.superuser.apiToken
}

func (n *netbox) Close(ctx context.Context) error {
	return n.g.Close(ctx)
}
