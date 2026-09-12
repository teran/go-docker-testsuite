package forgejo

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/pkg/errors"
	docker "github.com/teran/go-docker-testsuite"
)

const (
	// adminUsername is the login of the admin account created during setup.
	// Note: the literal name "admin" is a reserved username in Forgejo, so a
	// different name is used for the administrative account.
	adminUsername = "forgejo"
	adminPassword = "ForgejoTest123!"
)

// Forgejo exposes the minimal information a caller needs to talk to a running
// Forgejo instance: its web URL and the admin credentials created during the
// setup. The caller brings their own HTTP/API client — no Forgejo SDK
// dependency is embedded here (a deliberate decision to keep this stdlib-only).
type Forgejo interface {
	URL() (string, error)
	MustURL() string
	AdminUsername() string
	AdminPassword() string
	Close(ctx context.Context) error
}

type forgejo struct {
	c docker.Container
}

// config holds the consumer-tunable options for a Forgejo instance.
type config struct {
	disableRegistration bool
}

// Option configures a Forgejo instance.
type Option func(*config)

// WithDisableRegistration disables self-registration of new users
// (FORGEJO__service__DISABLE_REGISTRATION). Registration is left enabled by
// default; it is a policy decision of the consumer, unrelated to the setup
// bypass, so it is opt-in.
func WithDisableRegistration(disable bool) Option {
	return func(c *config) {
		c.disableRegistration = disable
	}
}

// env returns the environment that targets a SQLite-backed, install-locked
// Forgejo instance (no interactive onboarding).
//
// FORGEJO__security__INSTALL_LOCK is the mechanism that passes the initial
// setup screen: it locks the /install wizard so onboarding cannot be run, and
// the admin user created afterwards finalizes the database — the instance then
// redirects straight to login.
func env(cfg config) docker.Environment {
	e := docker.NewEnvironment().
		StringVar("FORGEJO__database__DB_TYPE", "sqlite3").
		StringVar("FORGEJO__database__PATH", "/data/forgejo/forgejo.db").
		StringVar("FORGEJO__security__INSTALL_LOCK", "true").
		StringVar("FORGEJO__server__ROOT_URL", "http://localhost:3000/").
		StringVar("FORGEJO__server__DOMAIN", "localhost").
		StringVar("FORGEJO__server__SSH_DOMAIN", "localhost")

	if cfg.disableRegistration {
		e = e.StringVar("FORGEJO__service__DISABLE_REGISTRATION", "true")
	}

	return e
}

// lifecycleOptions returns the lifecycle options for the Forgejo container.
//
// The Forgejo image runs its web process as the unprivileged "git" user and
// only chowns /data/gitea and /data/git on startup. Since this wrapper pins the
// SQLite database to /data/forgejo/forgejo.db, the /data/forgejo directory is
// created up front (the startup command runs as root via exec) and handed to
// the git user so the app can create its database there.
func lifecycleOptions() []docker.LifecycleOption {
	return []docker.LifecycleOption{
		docker.WithStartupCommand("/bin/sh", "-c", "mkdir -p /data/forgejo && chown git:git /data/forgejo"),
	}
}

func New(ctx context.Context, image string, opts ...Option) (Forgejo, error) {
	cfg := config{}
	for _, o := range opts {
		o(&cfg)
	}

	c, err := docker.NewContainerWithLifecycle(
		"forgejo",
		image,
		nil,
		env(cfg),
		docker.
			NewPortBindings().
			PortDNAT(docker.ProtoTCP, 3000),
		lifecycleOptions()...,
	)
	if err != nil {
		return nil, err
	}

	started := false
	defer func() {
		if !started {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_ = c.Close(cleanupCtx)
		}
	}()

	err = c.Run(ctx)
	if err != nil {
		return nil, err
	}

	err = c.AwaitOutput(ctx, docker.NewSubstringMatcher("Listen: http://0.0.0.0:3000"))
	if err != nil {
		return nil, err
	}

	if err := createAdminUser(ctx, c); err != nil {
		return nil, err
	}

	started = true
	return &forgejo{
		c: c,
	}, nil
}

// NewWithT is New bound to a *testing.T: the container's lifecycle is tied to
// the test and cleaned up automatically via t.Cleanup.
func NewWithT(t *testing.T, ctx context.Context, image string, opts ...Option) (Forgejo, error) {
	cfg := config{}
	for _, o := range opts {
		o(&cfg)
	}

	c, err := docker.NewContainerWithLifecycleT(
		t,
		"forgejo",
		image,
		nil,
		env(cfg),
		docker.
			NewPortBindings().
			PortDNAT(docker.ProtoTCP, 3000),
		lifecycleOptions()...,
	)
	if err != nil {
		return nil, err
	}

	started := false
	defer func() {
		if !started {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_ = c.Close(cleanupCtx)
		}
	}()

	err = c.Run(ctx)
	if err != nil {
		return nil, err
	}

	err = c.AwaitOutput(ctx, docker.NewSubstringMatcher("Listen: http://0.0.0.0:3000"))
	if err != nil {
		return nil, err
	}

	if err := createAdminUser(ctx, c); err != nil {
		return nil, err
	}

	started = true
	return &forgejo{
		c: c,
	}, nil
}

// createAdminUser creates the admin account once the web server is up, which
// finalizes the installation and bypasses the interactive onboarding screen
// (the install is already locked via FORGEJO__security__INSTALL_LOCK).
//
// The command is run via su-exec as the unprivileged "git" user: the Forgejo
// CLI refuses to run as root (it is meant to run as the same user as the web
// process).
func createAdminUser(ctx context.Context, c docker.Container) error {
	res, err := c.Exec(ctx, []string{
		"su-exec", "git", "forgejo", "admin", "user", "create",
		"--admin",
		"--username", adminUsername,
		"--password", adminPassword,
		"--email", "admin@localhost",
		"--must-change-password=false",
	})
	if err != nil {
		return errors.Wrap(err, "error creating admin user")
	}
	if res.ExitCode != 0 {
		return errors.Errorf(
			"error creating admin user (exit code %d): %s",
			res.ExitCode, string(res.Stderr),
		)
	}

	return nil
}

func (f *forgejo) URL() (string, error) {
	u, err := f.c.URL(docker.ProtoTCP, 3000)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("http://%s", u.String()), nil
}

func (f *forgejo) MustURL() string {
	u, err := f.URL()
	if err != nil {
		panic(err)
	}
	return u
}

func (f *forgejo) AdminUsername() string {
	return adminUsername
}

func (f *forgejo) AdminPassword() string {
	return adminPassword
}

func (f *forgejo) Close(ctx context.Context) error {
	return f.c.Close(ctx)
}
