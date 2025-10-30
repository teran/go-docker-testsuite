package postgres

import (
	"context"
	"fmt"
	"time"

	pgx "github.com/jackc/pgx/v4"

	"github.com/teran/go-docker-testsuite/v2"
	"github.com/teran/go-docker-testsuite/v2/images"
)

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

	return &postgresql{
		c: c,
	}, nil
}

func (p *postgresql) CreateDB(ctx context.Context, db string) error {
	dsn, err := p.DSN("")
	if err != nil {
		return err
	}

	pgconn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer func() { _ = pgconn.Close(ctx) }()

	// Arguments are not supported in CREATE DATABASE statement so using Sprintf() :(
	_, err = pgconn.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s", db))
	return err
}

func (p *postgresql) DropDB(ctx context.Context, db string) error {
	dsn, err := p.DSN("")
	if err != nil {
		return err
	}

	pgconn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer func() { _ = pgconn.Close(ctx) }()

	// Arguments are not supported in DROP DATABASE statement so using Sprintf() :(
	_, err = pgconn.Exec(ctx, fmt.Sprintf("DROP DATABASE %s", db))
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
