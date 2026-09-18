// Package clickhouse provides a typed wrapper around a ClickHouse container
// for integration testing.
//
// It manages the lifecycle of a single `clickhouse-server` container (via the
// official Docker SDK), waits for it to become ready (log line + driver ping),
// and exposes a typed client interface with DDL helpers (CreateDatabase /
// DropDatabase) and a connection-string accessor.
package clickhouse

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2" // registers the "clickhouse" driver

	"github.com/pkg/errors"
	docker "github.com/teran/go-docker-testsuite"
	"github.com/teran/go-docker-testsuite/images"
	wait "github.com/teran/go-docker-testsuite/wait"
)

const maxDBNameLen = 64

// validateDBName validates that name is a safe ClickHouse database identifier.
// Only printable ASCII letters, digits, and underscore are allowed to prevent
// Unicode normalization / homoglyph attacks. This follows the same
// ASCII-whitelist standard used by the other database wrappers (postgres,
// mysql, scylladb, mongodb).
func validateDBName(name string) error {
	if name == "" {
		return errors.New("database name must not be empty")
	}
	if len(name) > maxDBNameLen {
		return errors.Errorf("database name %q exceeds max length of %d bytes", name, maxDBNameLen)
	}

	for _, c := range name {
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '_':
		default:
			return errors.Errorf("invalid database name %q: character %q is not allowed", name, c)
		}
	}

	return nil
}

// ClickHouseUser and ClickHousePassword are the credentials the wrapper sets
// on the container and embeds in the DSN. ClickHouse disables network access
// for the default user unless a user/password is provided via the
// CLICKHOUSE_USER / CLICKHOUSE_PASSWORD environment variables.
const (
	ClickHouseUser     = "default"
	ClickHousePassword = "default"
)

// ClickHouse is the typed client interface exposed by the ClickHouse wrapper.
type ClickHouse interface {
	// DSN returns a ClickHouse connection string for the given database.
	DSN(db string) (string, error)
	// MustDSN is DSN that panics on error.
	MustDSN(db string) string
	// CreateDatabase creates a database.
	CreateDatabase(ctx context.Context, name string) error
	// DropDatabase drops a database.
	DropDatabase(ctx context.Context, name string) error
	// Close shuts down the ClickHouse client and the container.
	Close(ctx context.Context) error
}

type clickHouse struct {
	c docker.Container

	db *sql.DB
}

// New starts a ClickHouse container using the default image.
func New(ctx context.Context) (ClickHouse, error) {
	return NewWithImage(ctx, images.ClickHouse)
}

// NewWithImage starts a ClickHouse container using the given image.
func NewWithImage(ctx context.Context, image string) (ClickHouse, error) {
	c, err := docker.
		NewContainer(
			"clickhouse",
			image,
			nil,
			docker.NewEnvironment().
				StringVar("CLICKHOUSE_USER", ClickHouseUser).
				StringVar("CLICKHOUSE_PASSWORD", ClickHousePassword),
			docker.
				NewPortBindings().
				PortDNAT(docker.ProtoTCP, 9000),
		)
	if err != nil {
		return nil, errors.Wrap(err, "error creating new container")
	}

	app := &clickHouse{
		c: c,
	}

	started := false
	defer func() {
		if !started {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if app.db != nil {
				_ = app.db.Close()
			}
			_ = c.Close(cleanupCtx)
		}
	}()

	if err := c.Run(ctx); err != nil {
		return nil, errors.Wrap(err, "error running container")
	}

	// ClickHouse does not log readiness to stdout (its logs go to files), so
	// probe readiness by running clickhouse-client inside the container.
	if err := wait.Wait(ctx, c, wait.ForCommand([]string{"clickhouse-client", "--query", "SELECT 1"})); err != nil {
		return nil, errors.Wrap(err, "error waiting for ClickHouse to become ready")
	}

	dsn, err := app.DSN("")
	if err != nil {
		return nil, errors.Wrap(err, "error obtaining ClickHouse DSN")
	}

	db, err := sql.Open("clickhouse", dsn)
	if err != nil {
		return nil, errors.Wrap(err, "error connecting to ClickHouse")
	}
	app.db = db

	// Belt-and-braces: the log line may appear slightly before the driver can
	// actually complete a round-trip, so also ping until it succeeds.
	ready := false
	for i := 0; i < 30; i++ {
		if err := db.PingContext(ctx); err == nil {
			ready = true
			break
		}

		select {
		case <-ctx.Done():
			return nil, errors.Wrap(ctx.Err(), "context cancelled while waiting for ClickHouse ping")
		case <-time.After(time.Second):
		}
	}

	if !ready {
		return nil, errors.New("ClickHouse did not become ready: driver ping did not succeed within the retry window")
	}

	started = true
	return app, nil
}

// NewWithT is New bound to a *testing.T: the container's lifecycle is tied to
// the test and cleaned up automatically via t.Cleanup.
func NewWithT(t *testing.T, ctx context.Context) (ClickHouse, error) {
	return NewWithImageT(t, ctx, images.ClickHouse)
}

// NewWithImageT is NewWithImage bound to a *testing.T: the container's
// lifecycle is tied to the test and cleaned up automatically via t.Cleanup.
func NewWithImageT(t *testing.T, ctx context.Context, image string) (ClickHouse, error) {
	c, err := docker.
		NewContainerWithT(
			t,
			"clickhouse",
			image,
			nil,
			docker.NewEnvironment().
				StringVar("CLICKHOUSE_USER", ClickHouseUser).
				StringVar("CLICKHOUSE_PASSWORD", ClickHousePassword),
			docker.
				NewPortBindings().
				PortDNAT(docker.ProtoTCP, 9000),
		)
	if err != nil {
		return nil, errors.Wrap(err, "error creating new container")
	}

	app := &clickHouse{
		c: c,
	}

	started := false
	defer func() {
		if !started {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if app.db != nil {
				_ = app.db.Close()
			}
			_ = c.Close(cleanupCtx)
		}
	}()

	if err := c.Run(ctx); err != nil {
		return nil, errors.Wrap(err, "error running container")
	}

	if err := wait.Wait(ctx, c, wait.ForCommand([]string{"clickhouse-client", "--query", "SELECT 1"})); err != nil {
		return nil, errors.Wrap(err, "error waiting for ClickHouse to become ready")
	}

	dsn, err := app.DSN("")
	if err != nil {
		return nil, errors.Wrap(err, "error obtaining ClickHouse DSN")
	}

	db, err := sql.Open("clickhouse", dsn)
	if err != nil {
		return nil, errors.Wrap(err, "error connecting to ClickHouse")
	}
	app.db = db

	ready := false
	for i := 0; i < 30; i++ {
		if err := db.PingContext(ctx); err == nil {
			ready = true
			break
		}

		select {
		case <-ctx.Done():
			return nil, errors.Wrap(ctx.Err(), "context cancelled while waiting for ClickHouse ping")
		case <-time.After(time.Second):
		}
	}

	if !ready {
		return nil, errors.New("ClickHouse did not become ready: driver ping did not succeed within the retry window")
	}

	started = true
	return app, nil
}

func (c *clickHouse) DSN(db string) (string, error) {
	if db != "" {
		if err := validateDBName(db); err != nil {
			return "", err
		}
	}

	hp, err := c.c.URL(docker.ProtoTCP, 9000)
	if err != nil {
		return "", errors.Wrap(err, "error getting container URL")
	}

	return fmt.Sprintf("clickhouse://%s:%s@%s/%s", ClickHouseUser, ClickHousePassword, hp.String(), db), nil
}

func (c *clickHouse) MustDSN(db string) string {
	dsn, err := c.DSN(db)
	if err != nil {
		panic(err)
	}
	return dsn
}

// CreateDatabase creates a database via the driver API (injection-safe; the
// name is validated first as defence-in-depth).
func (c *clickHouse) CreateDatabase(ctx context.Context, name string) error {
	if err := validateDBName(name); err != nil {
		return err
	}

	if _, err := c.db.ExecContext(ctx, "CREATE DATABASE "+name); err != nil {
		return errors.Wrap(err, "error creating database")
	}

	return nil
}

// DropDatabase drops a database via the driver API (injection-safe; the name
// is validated first as defence-in-depth).
func (c *clickHouse) DropDatabase(ctx context.Context, name string) error {
	if err := validateDBName(name); err != nil {
		return err
	}

	if _, err := c.db.ExecContext(ctx, "DROP DATABASE "+name); err != nil {
		return errors.Wrap(err, "error dropping database")
	}

	return nil
}

func (c *clickHouse) Close(ctx context.Context) error {
	var err error
	if c.db != nil {
		err = c.db.Close()
	}

	if cerr := c.c.Close(ctx); cerr != nil && err == nil {
		err = cerr
	}

	return errors.Wrap(err, "error closing ClickHouse")
}
