// Package mongodb provides a typed wrapper around a MongoDB container for
// integration testing.
//
// It manages the lifecycle of a single `mongo` container (via the official
// Docker SDK), waits for it to become ready (log line + driver ping), and
// exposes a typed client interface with DDL helpers (CreateDatabase /
// DropDatabase) and a connection-URI accessor.
package mongodb

import (
	"context"
	"fmt"
	"testing"
	"time"
	"unicode"

	mongoClient "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	docker "github.com/teran/go-docker-testsuite"
	"github.com/teran/go-docker-testsuite/images"
	wait "github.com/teran/go-docker-testsuite/wait"
)

const maxDBNameLen = 64

// validateDBName validates that name is a safe MongoDB database name.
//
// MongoDB database names must not contain `/ \ . " $ * < > : | ?` or null
// bytes, must not begin with `.` or `$`, and are limited to 64 bytes (see
// https://www.mongodb.com/docs/manual/reference/limits/#mongodb-limit-Database-Name).
//
// To prevent URI injection and Unicode normalization / homoglyph attacks we
// whitelist only Unicode letters, digits, underscore and hyphen — which
// automatically excludes every disallowed character (including `/`, which
// would otherwise break the connection-URI path) and prevents names starting
// with `.` or `$`.
func validateDBName(name string) error {
	if name == "" {
		return errors.New("database name must not be empty")
	}
	if len(name) > maxDBNameLen {
		return errors.Errorf("database name %q exceeds max length of %d bytes", name, maxDBNameLen)
	}

	for _, c := range name {
		switch {
		case unicode.IsLetter(c), unicode.IsDigit(c):
		case c == '_', c == '-':
		default:
			return errors.Errorf("invalid database name %q: character %q is not allowed", name, c)
		}
	}

	return nil
}

// Mongo is the typed client interface exposed by the MongoDB wrapper.
type Mongo interface {
	// URI returns a mongodb:// connection string for the given database
	// (empty name yields a connection string without a default database).
	URI(db string) (string, error)
	// MustURI is URI that panics on error.
	MustURI(db string) string
	// CreateDatabase materializes a database (MongoDB has no CREATE DATABASE
	// statement — see implementation notes).
	CreateDatabase(ctx context.Context, name string) error
	// DropDatabase drops a database.
	DropDatabase(ctx context.Context, name string) error
	// Close shuts down the Mongo client and the container.
	Close(ctx context.Context) error
}

type mongo struct {
	c docker.Container

	client *mongoClient.Client
}

// New starts a MongoDB container using the default image.
func New(ctx context.Context) (Mongo, error) {
	return NewWithImage(ctx, images.MongoDB)
}

// NewWithImage starts a MongoDB container using the given image.
func NewWithImage(ctx context.Context, image string) (Mongo, error) {
	c, err := docker.
		NewContainer(
			"mongodb",
			image,
			nil,
			docker.NewEnvironment(),
			docker.
				NewPortBindings().
				PortDNAT(docker.ProtoTCP, 27017),
		)
	if err != nil {
		return nil, errors.Wrap(err, "error creating new container")
	}

	app := &mongo{
		c: c,
	}

	started := false
	defer func() {
		if !started {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if app.client != nil {
				_ = app.client.Disconnect(cleanupCtx)
			}
			_ = c.Close(cleanupCtx)
		}
	}()

	if err := c.Run(ctx); err != nil {
		return nil, errors.Wrap(err, "error running container")
	}

	// "Waiting for connections" is emitted by mongod once it is ready to
	// accept client connections and is stable across MongoDB 7.x and 8.x
	// (the structured-log "msg" field has kept this wording since 4.4).
	if err := wait.Wait(ctx, c, wait.ForLog(docker.NewSubstringMatcher("Waiting for connections"))); err != nil {
		return nil, errors.Wrap(err, "error waiting for MongoDB to become ready")
	}

	uri, err := app.URI("")
	if err != nil {
		return nil, errors.Wrap(err, "error obtaining MongoDB connection URI")
	}

	client, err := mongoClient.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, errors.Wrap(err, "error connecting to MongoDB")
	}
	app.client = client

	// Belt-and-braces: the log line may appear slightly before the driver can
	// actually complete a round-trip, so also ping until it succeeds.
	ready := false
	for i := 0; i < 30; i++ {
		if err := client.Ping(ctx, nil); err == nil {
			ready = true
			break
		}

		log.Debug("MongoDB is not ready yet. Awaiting for ping to pass ...")

		select {
		case <-ctx.Done():
			return nil, errors.Wrap(ctx.Err(), "context cancelled while waiting for MongoDB ping")
		case <-time.After(time.Second):
		}
	}

	if !ready {
		return nil, errors.New("MongoDB did not become ready: driver ping did not succeed within the retry window")
	}

	started = true
	return app, nil
}

// NewWithT is New bound to a *testing.T: the container's lifecycle is tied to
// the test and cleaned up automatically via t.Cleanup.
func NewWithT(t *testing.T, ctx context.Context) (Mongo, error) {
	return NewWithImageT(t, ctx, images.MongoDB)
}

// NewWithImageT is NewWithImage bound to a *testing.T: the container's
// lifecycle is tied to the test and cleaned up automatically via t.Cleanup.
func NewWithImageT(t *testing.T, ctx context.Context, image string) (Mongo, error) {
	c, err := docker.
		NewContainerWithT(
			t,
			"mongodb",
			image,
			nil,
			docker.NewEnvironment(),
			docker.
				NewPortBindings().
				PortDNAT(docker.ProtoTCP, 27017),
		)
	if err != nil {
		return nil, errors.Wrap(err, "error creating new container")
	}

	app := &mongo{
		c: c,
	}

	started := false
	defer func() {
		if !started {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if app.client != nil {
				_ = app.client.Disconnect(cleanupCtx)
			}
			_ = c.Close(cleanupCtx)
		}
	}()

	if err := c.Run(ctx); err != nil {
		return nil, errors.Wrap(err, "error running container")
	}

	if err := wait.Wait(ctx, c, wait.ForLog(docker.NewSubstringMatcher("Waiting for connections"))); err != nil {
		return nil, errors.Wrap(err, "error waiting for MongoDB to become ready")
	}

	uri, err := app.URI("")
	if err != nil {
		return nil, errors.Wrap(err, "error obtaining MongoDB connection URI")
	}

	client, err := mongoClient.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, errors.Wrap(err, "error connecting to MongoDB")
	}
	app.client = client

	// Belt-and-braces: the log line may appear slightly before the driver can
	// actually complete a round-trip, so also ping until it succeeds.
	ready := false
	for i := 0; i < 30; i++ {
		if err := client.Ping(ctx, nil); err == nil {
			ready = true
			break
		}

		log.Debug("MongoDB is not ready yet. Awaiting for ping to pass ...")

		select {
		case <-ctx.Done():
			return nil, errors.Wrap(ctx.Err(), "context cancelled while waiting for MongoDB ping")
		case <-time.After(time.Second):
		}
	}

	if !ready {
		return nil, errors.New("MongoDB did not become ready: driver ping did not succeed within the retry window")
	}

	started = true
	return app, nil
}

func (m *mongo) URI(db string) (string, error) {
	if db != "" {
		if err := validateDBName(db); err != nil {
			return "", err
		}
	}

	hp, err := m.c.URL(docker.ProtoTCP, 27017)
	if err != nil {
		return "", errors.Wrap(err, "error getting container URL")
	}

	uri := fmt.Sprintf("mongodb://%s:%d/%s", hp.Host, hp.Port, db)

	log.Tracef("MongoDB URI: %s", uri)

	return uri, nil
}

func (m *mongo) MustURI(db string) string {
	uri, err := m.URI(db)
	if err != nil {
		panic(err)
	}
	return uri
}

// CreateDatabase materializes a database. MongoDB has no CREATE DATABASE
// statement — databases are created lazily on first write. Creating a
// collection on the target database is the cleanest reliable way to force the
// database into existence, and since it uses the driver API (no string
// interpolation) it is inherently injection-safe. The database name is still
// validated first as defence-in-depth.
func (m *mongo) CreateDatabase(ctx context.Context, name string) error {
	if err := validateDBName(name); err != nil {
		return err
	}

	if err := m.client.Database(name).CreateCollection(ctx, "_created"); err != nil {
		return errors.Wrap(err, "error creating database")
	}

	return nil
}

// DropDatabase drops a database via the driver API (injection-safe).
func (m *mongo) DropDatabase(ctx context.Context, name string) error {
	if err := validateDBName(name); err != nil {
		return err
	}

	if err := m.client.Database(name).Drop(ctx); err != nil {
		return errors.Wrap(err, "error dropping database")
	}

	return nil
}

func (m *mongo) Close(ctx context.Context) error {
	var err error
	if m.client != nil {
		err = m.client.Disconnect(ctx)
	}

	if cerr := m.c.Close(ctx); cerr != nil && err == nil {
		err = cerr
	}

	return errors.Wrap(err, "error closing MongoDB")
}
