// Package paperlessngx provides a Paperless-ngx instance for integration
// testing.
//
// Paperless-ngx cannot run on SQLite in this wrapper — it requires an external
// PostgreSQL database and a Valkey/Redis message broker — so this wrapper runs
// a docker.Group on an internal network where the siblings resolve each other
// by container name:
//
//	paperless-postgres (PostgreSQL, internal only)
//	paperless-valkey   (Valkey / Redis-protocol broker, internal only)
//	paperless          (the Paperless-ngx web server, port 8000 DNAT'd to the host)
//
// Two additional containers are spun up only when their corresponding Option
// is set, and are used exclusively at document-consumption time (the web server
// boots regardless of them):
//
//	paperless-gotenberg (Gotenberg, office → PDF conversion, internal only)
//	paperless-tika      (Apache Tika, office text extraction, internal only)
//
// The caller brings their own HTTP client; no Paperless-ngx SDK dependency is
// embedded here (a deliberate decision to keep this stdlib-only).
package paperlessngx

import (
	"context"
	"encoding/base64"
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
	// WithSecretKey when a real secret is needed. Paperless-ngx refuses to start
	// without PAPERLESS_SECRET_KEY.
	// #nosec G101 -- intentionally-insecure fixed test secret, documented above.
	defaultSecretKey = "go-docker-testsuite-paperlessngx-insecure-test-only-secret"

	// defaultDBPassword is a fixed test-only password used for the backing
	// PostgreSQL. Override with WithDBPassword.
	defaultDBPassword = "paperless-test-password"

	// Default admin credentials created during setup. They are deterministic so
	// tests can rely on them without extra configuration.
	defaultAdminUsername = "admin"
	defaultAdminPassword = "PaperlessTest123!"
	defaultAdminEmail    = "admin@example.com"

	// Container names on the internal network. The Paperless-ngx container
	// reaches its dependencies by these DNS names.
	containerPostgres  = "paperless-postgres"
	containerValkey    = "paperless-valkey"
	containerPaperless = "paperless"
	containerGotenberg = "paperless-gotenberg"
	containerTika      = "paperless-tika"

	dbName        = "paperless"
	dbUser        = "paperless"
	dbPort        = "5432"
	valkeyPort    = "6379"
	gotenbergPort = "3000"
	tikaPort      = "9998"

	webPort = 8000
)

// PaperlessNGX exposes the minimal information a caller needs to talk to a
// running Paperless-ngx instance: its web URL and the admin credentials created
// during setup. The caller brings their own HTTP/API client — no Paperless-ngx
// SDK dependency is embedded here (a deliberate decision to keep this
// stdlib-only).
type PaperlessNGX interface {
	URL() (string, error)
	MustURL() string
	Username() string
	Password() string
	Close(ctx context.Context) error
}

type paperlessngx struct {
	g   docker.Group
	c   docker.Container
	cfg config
}

// adminConfig carries the credentials of the Paperless-ngx admin that is
// created on first startup.
type adminConfig struct {
	username string
	password string
	email    string
}

// config holds the consumer-tunable options for a Paperless-ngx instance.
type config struct {
	secretKey    string
	dbPassword   string
	allowedHosts []string
	admin        *adminConfig
	gotenberg    bool
	tika         bool
}

// Option configures a Paperless-ngx instance.
type Option func(*config)

// WithSecretKey sets the Paperless-ngx secret key (PAPERLESS_SECRET_KEY).
// Defaults to a fixed, deterministic and deliberately insecure test-only value —
// override for anything that must not use a known secret. The server refuses to
// start without a secret key.
func WithSecretKey(secretKey string) Option {
	return func(c *config) {
		c.secretKey = secretKey
	}
}

// WithDBPassword sets the password used for the backing PostgreSQL instance
// (POSTGRES_PASSWORD / PAPERLESS_DBPASS). Defaults to a fixed test-only constant.
func WithDBPassword(password string) Option {
	return func(c *config) {
		c.dbPassword = password
	}
}

// WithAdmin overrides the admin account created during setup
// (PAPERLESS_ADMIN_USER / PAPERLESS_ADMIN_PASSWORD / PAPERLESS_ADMIN_MAIL).
// Defaults to a deterministic test account ("admin").
func WithAdmin(username, password, email string) Option {
	return func(c *config) {
		c.admin = &adminConfig{
			username: username,
			password: password,
			email:    email,
		}
	}
}

// WithAllowedHosts sets PAPERLESS_ALLOWED_HOSTS (comma-joined). When unset the
// Paperless-ngx default ("*") is left in place, so the instance answers on any
// host.
func WithAllowedHosts(hosts ...string) Option {
	return func(c *config) {
		c.allowedHosts = hosts
	}
}

// WithGotenberg enables the optional Gotenberg container (office → PDF
// conversion) and wires PAPERLESS_GOTENBERG_ENDPOINT so Paperless-ngx can use it
// during document consumption.
func WithGotenberg() Option {
	return func(c *config) {
		c.gotenberg = true
	}
}

// WithTika enables the optional Apache Tika container (office text extraction)
// and wires PAPERLESS_TIKA_ENABLED / PAPERLESS_TIKA_ENDPOINT so Paperless-ngx can
// use it during document consumption.
func WithTika() Option {
	return func(c *config) {
		c.tika = true
	}
}

func defaultConfig() config {
	return config{
		secretKey:  defaultSecretKey,
		dbPassword: defaultDBPassword,
		admin: &adminConfig{
			username: defaultAdminUsername,
			password: defaultAdminPassword,
			email:    defaultAdminEmail,
		},
	}
}

// env returns the environment for the Paperless-ngx web server container. The
// exact variable names are critical — Paperless-ngx reads these to connect to
// PostgreSQL, Valkey and to create the admin account on first boot.
func env(cfg config) docker.Environment {
	e := docker.NewEnvironment().
		StringVar("PAPERLESS_SECRET_KEY", cfg.secretKey).
		StringVar("PAPERLESS_REDIS", "redis://"+containerValkey+":"+valkeyPort).
		StringVar("PAPERLESS_DBENGINE", "postgresql").
		StringVar("PAPERLESS_DBHOST", containerPostgres).
		StringVar("PAPERLESS_DBPORT", dbPort).
		StringVar("PAPERLESS_DBNAME", dbName).
		StringVar("PAPERLESS_DBUSER", dbUser).
		StringVar("PAPERLESS_DBPASS", cfg.dbPassword).
		StringVar("PAPERLESS_ADMIN_USER", cfg.admin.username).
		StringVar("PAPERLESS_ADMIN_PASSWORD", cfg.admin.password).
		StringVar("PAPERLESS_ADMIN_MAIL", cfg.admin.email)

	if len(cfg.allowedHosts) > 0 {
		e = e.StringVar("PAPERLESS_ALLOWED_HOSTS", strings.Join(cfg.allowedHosts, ","))
	}

	if cfg.gotenberg {
		e = e.StringVar("PAPERLESS_GOTENBERG_ENDPOINT", "http://"+containerGotenberg+":"+gotenbergPort)
	}

	if cfg.tika {
		e = e.
			StringVar("PAPERLESS_TIKA_ENABLED", "true").
			StringVar("PAPERLESS_TIKA_ENDPOINT", "http://"+containerTika+":"+tikaPort)
	}

	return e
}

// postgresReady is an AfterRun hook that blocks until PostgreSQL accepts
// connections, so the Paperless-ngx container (started later in the group)
// finds a live database.
func postgresReady(ctx context.Context, ht docker.HookType, c docker.Container) error {
	if ht == docker.HookTypeAfterRun {
		return c.AwaitOutput(ctx, docker.NewSubstringMatcher("database system is ready to accept connections"))
	}
	return nil
}

// valkeyReady is an AfterRun hook that blocks until Valkey accepts connections.
func valkeyReady(ctx context.Context, ht docker.HookType, c docker.Container) error {
	if ht == docker.HookTypeAfterRun {
		return c.AwaitOutput(ctx, docker.NewSubstringMatcher("Ready to accept connections"))
	}
	return nil
}

// New starts a Paperless-ngx instance backed by its own PostgreSQL and Valkey
// containers on a shared internal network.
func New(ctx context.Context, image string, opts ...Option) (PaperlessNGX, error) {
	return newPaperlessNGX(ctx, nil, image, opts...)
}

// NewWithT is New bound to a *testing.T: the containers' lifecycles are tied to
// the test and cleaned up automatically via t.Cleanup.
func NewWithT(t *testing.T, ctx context.Context, image string, opts ...Option) (PaperlessNGX, error) {
	return newPaperlessNGX(ctx, t, image, opts...)
}

func newPaperlessNGX(ctx context.Context, t *testing.T, image string, opts ...Option) (PaperlessNGX, error) {
	cfg := defaultConfig()
	for _, o := range opts {
		o(&cfg)
	}

	pgEnv := docker.NewEnvironment().
		StringVar("POSTGRES_DB", dbName).
		StringVar("POSTGRES_USER", dbUser).
		StringVar("POSTGRES_PASSWORD", cfg.dbPassword)

	valkeyEnv := docker.NewEnvironment()

	mainEnv := env(cfg)

	var (
		pgC     docker.Container
		valkeyC docker.Container
		mainC   docker.Container
		gotenC  docker.Container
		tikaC   docker.Container
		err     error
	)

	if t != nil {
		pgC, err = docker.NewContainerWithT(t, containerPostgres, images.PaperlessNGXPostgres, nil, pgEnv, docker.NewPortBindings())
		if err != nil {
			return nil, err
		}
		valkeyC, err = docker.NewContainerWithT(t, containerValkey, images.PaperlessNGXValkey, nil, valkeyEnv, docker.NewPortBindings())
		if err != nil {
			return nil, err
		}
		mainC, err = docker.NewContainerWithT(t, containerPaperless, image, nil, mainEnv, docker.NewPortBindings().PortDNAT(docker.ProtoTCP, webPort))
		if err != nil {
			return nil, err
		}
		if cfg.gotenberg {
			gotenC, err = docker.NewContainerWithT(t, containerGotenberg, images.PaperlessNGXGotenberg, nil, docker.NewEnvironment(), docker.NewPortBindings())
			if err != nil {
				return nil, err
			}
		}
		if cfg.tika {
			tikaC, err = docker.NewContainerWithT(t, containerTika, images.PaperlessNGXTika, nil, docker.NewEnvironment(), docker.NewPortBindings())
			if err != nil {
				return nil, err
			}
		}
	} else {
		pgC, err = docker.NewContainer(containerPostgres, images.PaperlessNGXPostgres, nil, pgEnv, docker.NewPortBindings())
		if err != nil {
			return nil, err
		}
		valkeyC, err = docker.NewContainer(containerValkey, images.PaperlessNGXValkey, nil, valkeyEnv, docker.NewPortBindings())
		if err != nil {
			return nil, err
		}
		mainC, err = docker.NewContainer(containerPaperless, image, nil, mainEnv, docker.NewPortBindings().PortDNAT(docker.ProtoTCP, webPort))
		if err != nil {
			return nil, err
		}
		if cfg.gotenberg {
			gotenC, err = docker.NewContainer(containerGotenberg, images.PaperlessNGXGotenberg, nil, docker.NewEnvironment(), docker.NewPortBindings())
			if err != nil {
				return nil, err
			}
		}
		if cfg.tika {
			tikaC, err = docker.NewContainer(containerTika, images.PaperlessNGXTika, nil, docker.NewEnvironment(), docker.NewPortBindings())
			if err != nil {
				return nil, err
			}
		}
	}

	apps := []*docker.Application{
		docker.NewApplication(pgC, postgresReady),
		docker.NewApplication(valkeyC, valkeyReady),
	}
	if cfg.gotenberg {
		apps = append(apps, docker.NewApplication(gotenC))
	}
	if cfg.tika {
		apps = append(apps, docker.NewApplication(tikaC))
	}
	apps = append(apps, docker.NewApplication(mainC))

	g, err := docker.NewGroup("paperless-ngx", apps...)
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

	n := &paperlessngx{g: g, c: mainC, cfg: cfg}

	if err := n.awaitReady(ctx); err != nil {
		return nil, err
	}

	started = true
	return n, nil
}

// awaitReady implements the two-phase readiness check for the Paperless-ngx
// container.
//
// Phase A polls the web root until it answers HTTP 200 — this mirrors the
// container's own healthcheck (Paperless-ngx has no reliable single "ready"
// log line to wait on).
//
// Phase B (belt-and-braces) verifies the admin credentials are usable by
// requesting the API root with Basic auth, guaranteeing the credentials exposed
// by the wrapper actually work.
//
// The whole check is bounded by a 10-minute timeout; on expiry the wrapped
// error reports that Paperless-ngx never became ready. This is deliberately
// generous: the first start runs database migrations and model/index setup
// against a fresh PostgreSQL, which can take several minutes on hosts with slow
// disk I/O (e.g. Docker Desktop on macOS).
func (n *paperlessngx) awaitReady(ctx context.Context) error {
	readyCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	u, err := n.c.URL(docker.ProtoTCP, webPort)
	if err != nil {
		return errors.Wrap(err, "paperless-ngx did not become ready")
	}

	baseURL := fmt.Sprintf("http://%s", u.String())

	// Phase A: poll the web root until it returns HTTP 200.
	if err := n.probeHTTP(readyCtx, baseURL); err != nil {
		return errors.Wrap(err, "paperless-ngx did not become ready")
	}

	// Phase B: verify the admin credentials against the API root.
	if err := n.probeAPI(readyCtx, baseURL); err != nil {
		return errors.Wrap(err, "paperless-ngx did not become ready")
	}

	return nil
}

// probeHTTP polls url until an unauthenticated GET returns HTTP 200, or the
// parent context (the awaitReady budget) expires. The first boot runs DB
// migrations and model/index setup, which can take several minutes, so this
// must keep polling for the full budget rather than a short fixed deadline.
func (n *paperlessngx) probeHTTP(ctx context.Context, url string) error {
	client := &http.Client{Timeout: 5 * time.Second}

	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}

		resp, err := client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return errors.Wrap(ctx.Err(), "web root did not answer HTTP 200 within the timeout")
		case <-time.After(time.Second):
		}
	}
}

// probeAPI verifies the admin credentials by requesting the API root with Basic
// auth and expecting HTTP 200, polling until the parent context expires.
func (n *paperlessngx) probeAPI(ctx context.Context, baseURL string) error {
	client := &http.Client{Timeout: 5 * time.Second}

	token := base64.StdEncoding.EncodeToString([]byte(n.Username() + ":" + n.Password()))

	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/", nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Basic "+token)

		resp, err := client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return errors.Wrap(ctx.Err(), "admin credentials were not accepted by the API within the timeout")
		case <-time.After(time.Second):
		}
	}
}

func (n *paperlessngx) URL() (string, error) {
	u, err := n.c.URL(docker.ProtoTCP, webPort)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("http://%s", u.String()), nil
}

func (n *paperlessngx) MustURL() string {
	u, err := n.URL()
	if err != nil {
		panic(err)
	}
	return u
}

func (n *paperlessngx) Username() string {
	return n.cfg.admin.username
}

func (n *paperlessngx) Password() string {
	return n.cfg.admin.password
}

func (n *paperlessngx) Close(ctx context.Context) error {
	return n.g.Close(ctx)
}
