package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	pgx "github.com/jackc/pgx/v4"

	"github.com/teran/go-docker-testsuite"
	"github.com/teran/go-docker-testsuite/images"
)

const maxDBNameLen = 63

// validateDBName validates that name is a safe PostgreSQL unquoted identifier.
// PostgreSQL allows: letters (including Unicode), underscore, digits, and dollar signs ($).
// See https://www.postgresql.org/docs/current/sql-syntax-lexical.html#SQL-SYNTAX-IDENTIFIERS
func validateDBName(name string) error {
	if name == "" {
		return fmt.Errorf("database name must not be empty")
	}
	if len(name) > maxDBNameLen {
		return fmt.Errorf("database name %q exceeds max length of %d bytes", name, maxDBNameLen)
	}

	r := []rune(name)
	if r[0] != '_' && !unicode.IsLetter(r[0]) {
		return fmt.Errorf("invalid database name %q: must start with a letter or underscore", name)
	}

	for _, c := range r {
		if c != '_' && c != '$' && !unicode.IsLetter(c) && !unicode.IsDigit(c) {
			return fmt.Errorf("invalid database name %q: character %q is not allowed", name, c)
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
			_ = c.Close(ctx)
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

	// NB: give it some time to assign a port
	time.Sleep(1 * time.Second)

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
