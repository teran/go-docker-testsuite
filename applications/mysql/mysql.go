package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	docker "github.com/teran/go-docker-testsuite"
)

type MySQL interface {
	Close(ctx context.Context) error
	CreateDB(ctx context.Context, name string) error
	DropDB(ctx context.Context, name string) error
	DSN(name string) (string, error)
	MustDSN(name string) string
}

type mysql struct {
	c docker.Container
}

func New(ctx context.Context, image string) (MySQL, error) {
	c, err := docker.NewContainer(
		"mysql",
		image,
		nil,
		docker.
			NewEnvironment().
			StringVar("MYSQL_ALLOW_EMPTY_PASSWORD", "true"),
		docker.
			NewPortBindings().
			PortDNAT(docker.ProtoTCP, 3306),
	)
	if err != nil {
		return nil, errors.Wrap(err, "error creating new container")
	}

	app := &mysql{
		c: c,
	}

	if err := c.Run(ctx); err != nil {
		return nil, errors.Wrap(err, "error running container")
	}

	re, err := regexp.Compile(
		`(mysqld|mariadbd):\s+ready\s+for\s+connections\.`,
	)
	if err != nil {
		return nil, errors.Wrap(err, "error compiling regex")
	}

	if err := c.AwaitOutput(ctx, docker.NewRegexpMatcher(re)); err != nil {
		return nil, errors.Wrap(err, "error awaiting container output")
	}

	dsn, err := app.DSN("")
	if err != nil {
		return nil, errors.Wrap(err, "error obtaining database DSN")
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, errors.Wrap(err, "error opening database connection")
	}
	defer db.Close()

	for i := 0; i < 30; i++ {
		if err := db.Ping(); err == nil {
			break
		}

		log.Debug("Database is not ready yet. Awaiting for ping to pass ...")

		time.Sleep(1 * time.Second)
	}

	return app, nil
}

func (m *mysql) CreateDB(ctx context.Context, name string) error {
	dsn, err := m.DSN("")
	if err != nil {
		return errors.Wrap(err, "error obtaining database DSN")
	}

	c, err := sql.Open("mysql", dsn)
	if err != nil {
		return errors.Wrap(err, "error opening database connection")
	}
	defer c.Close()

	if err := c.Ping(); err != nil {
		return errors.Wrap(err, "error pinging database")
	}

	_, err = c.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE %s", name))
	return errors.Wrap(err, "error executing SQL query")
}

func (m *mysql) DropDB(ctx context.Context, name string) error {
	dsn, err := m.DSN("")
	if err != nil {
		return errors.Wrap(err, "error obtaining database DSN")
	}

	c, err := sql.Open("mysql", dsn)
	if err != nil {
		return errors.Wrap(err, "error opening database connection")
	}
	defer c.Close()

	if err := c.Ping(); err != nil {
		return errors.Wrap(err, "error pinging database")
	}

	_, err = c.ExecContext(ctx, fmt.Sprintf("DROP DATABASE %s", name))
	return errors.Wrap(err, "error executing SQL query")
}

func (m *mysql) DSN(name string) (string, error) {
	hp, err := m.c.URL(docker.ProtoTCP, 3306)
	if err != nil {
		return "", errors.Wrap(err, "error getting container URL")
	}

	dsn := fmt.Sprintf("root@tcp(%s:%d)/%s", hp.Host, hp.Port, name)

	log.Tracef("DSN: %s", dsn)

	return dsn, nil
}

func (m *mysql) MustDSN(name string) string {
	dsn, err := m.DSN(name)
	if err != nil {
		panic(err)
	}
	return dsn
}

func (m *mysql) Close(ctx context.Context) error {
	return errors.Wrap(m.c.Close(ctx), "error closing database connection")
}
