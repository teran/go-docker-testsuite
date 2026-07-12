package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	pgx "github.com/jackc/pgx/v4"

	"github.com/pkg/errors"
	"github.com/teran/go-docker-testsuite"
	"github.com/teran/go-docker-testsuite/images"
)

const maxDBNameLen = 63

// validateDBName validates that name is a safe PostgreSQL identifier.
// Only printable ASCII letters, digits, and underscore are allowed to
// prevent Unicode normalization / homoglyph attacks. Note that `$` is
// NOT valid in PostgreSQL unquoted identifiers (it is only allowed inside
// dollar-quoted string constants).
// See https://www.postgresql.org/docs/current/sql-syntax-lexical.html#SQL-SYNTAX-IDENTIFIERS
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

// quotePGIdentifier wraps name in double quotes, escaping embedded quotes by doubling.
func quotePGIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

type PostgreSQL interface {
	DSN(db string) (string, error)
	MustDSN(db string) string
	CreateDB(ctx context.Context, db string) error
	DropDB(ctx context.Context, db string) error
	Close(ctx context.Context) error
}

type postgresql struct {
	c docker.Container
}

func New(ctx context.Context) (PostgreSQL, error) {
	return NewWithImage(ctx, images.Postgres)
}

func NewWithImage(ctx context.Context, image string) (PostgreSQL, error) {
	c, err := docker.
		NewContainer(
			"postgres",
			image,
			nil,
			docker.
				NewEnvironment().
				StringVar("POSTGRES_HOST_AUTH_METHOD", "trust"),
			docker.
				NewPortBindings().
				PortDNAT(docker.ProtoTCP, 5432),
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

	err = c.AwaitOutput(ctx, docker.NewSubstringMatcher("database system is ready to accept connections"))
	if err != nil {
		return nil, err
	}

	hp, err := c.URL(docker.ProtoTCP, 5432)
	if err != nil {
		return nil, err
	}

	dsn := fmt.Sprintf("postgres://postgres@%s/%s?sslmode=disable", hp.String(), "postgres")

	// Wait for PostgreSQL to accept TCP connections with a retry loop
	// instead of a blind sleep, so startup is fast on fast hosts and
	// resilient on loaded ones.
	for i := 0; i < 30; i++ {
		pgconn, pgErr := pgx.Connect(ctx, dsn)
		if pgErr == nil {
			_ = pgconn.Close(ctx)
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}

	started = true
	return &postgresql{
		c: c,
	}, nil
}

func (p *postgresql) CreateDB(ctx context.Context, db string) error {
	if err := validateDBName(db); err != nil {
		return err
	}

	dsn, err := p.DSN("")
	if err != nil {
		return err
	}

	pgconn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer func() { _ = pgconn.Close(ctx) }()

	// Arguments are not supported in CREATE DATABASE statement so using Sprintf()
	// with double-quote escaping as defence-in-depth. See validateDBName() for
	// the primary validation layer.
	_, err = pgconn.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s", quotePGIdentifier(db)))
	return err
}

func (p *postgresql) DropDB(ctx context.Context, db string) error {
	if err := validateDBName(db); err != nil {
		return err
	}

	dsn, err := p.DSN("")
	if err != nil {
		return err
	}

	pgconn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer func() { _ = pgconn.Close(ctx) }()

	// Arguments are not supported in DROP DATABASE statement so using Sprintf()
	// with double-quote escaping as defence-in-depth. See validateDBName() for
	// the primary validation layer.
	_, err = pgconn.Exec(ctx, fmt.Sprintf("DROP DATABASE %s", quotePGIdentifier(db)))
	return err
}

func (p *postgresql) DSN(db string) (string, error) {
	hp, err := p.c.URL(docker.ProtoTCP, 5432)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("postgres://postgres@%s/%s?sslmode=disable", hp.String(), db), nil
}

func (p *postgresql) MustDSN(db string) string {
	dsn, err := p.DSN(db)
	if err != nil {
		panic(err)
	}
	return dsn
}

func (p *postgresql) Close(ctx context.Context) error {
	return p.c.Close(ctx)
}
