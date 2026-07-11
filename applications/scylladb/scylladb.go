package scylladb

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/gocql/gocql"

	"github.com/teran/go-docker-testsuite"
	"github.com/teran/go-docker-testsuite/images"
)

const maxKeyspaceNameLen = 48

// validateKeyspaceName validates that name is a safe CQL identifier.
// CQL identifiers allow: letters, underscore, and digits.
// ScyllaDB keyspace names are limited to 48 characters.
// See https://docs.scylladb.com/stable/cql/ddl.html
func validateKeyspaceName(name string) error {
	if name == "" {
		return fmt.Errorf("keyspace name must not be empty")
	}
	if len(name) > maxKeyspaceNameLen {
		return fmt.Errorf("keyspace name %q exceeds max length of %d characters", name, maxKeyspaceNameLen)
	}

	r := []rune(name)
	if r[0] != '_' && !unicode.IsLetter(r[0]) {
		return fmt.Errorf("invalid keyspace name %q: must start with a letter or underscore", name)
	}

	for _, c := range r {
		if c != '_' && !unicode.IsLetter(c) && !unicode.IsDigit(c) {
			return fmt.Errorf("invalid keyspace name %q: character %q is not allowed", name, c)
		}
	}

	return nil
}

// quoteCQLIdentifier wraps name in double quotes, escaping embedded quotes by doubling.
func quoteCQLIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

type ScyllaDB interface {
	ClusterConfig(keyspaceName string) (*gocql.ClusterConfig, error)
	CreateKeyspace(name string) error
	DropKeyspace(name string) error
	Close(context.Context) error
}

type scylladb struct {
	c       docker.Container
	session *gocql.Session
}

func New(ctx context.Context) (ScyllaDB, error) {
	return NewWithImage(ctx, images.ScyllaDB)
}

func NewWithImage(ctx context.Context, image string) (ScyllaDB, error) {
	c, err := docker.
		NewContainer(
			"scylladb",
			image,
			[]string{
				"--overprovisioned=1",
				"--memory=1G",
				"--smp=1",
				"--developer-mode=1",
				"--idle-poll-time-us=0",
				"--poll-aio 0",
			},
			docker.NewEnvironment(),
			docker.NewPortBindings().
				PortDNAT(docker.ProtoTCP, 9042),
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

	ipRe := `\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\`
	l := fmt.Sprintf(
		`\] cql_server_controller - Starting listening for CQL clients on %s:9042 \(unencrypted, non-shard-aware\)$`,
		ipRe,
	)
	re, err := regexp.Compile(l)
	if err != nil {
		return nil, err
	}

	err = c.AwaitOutput(ctx, docker.NewRegexpMatcher(re))
	if err != nil {
		return nil, err
	}

	sd := &scylladb{
		c: c,
	}

	cfg, err := sd.ClusterConfig("")
	if err != nil {
		return nil, err
	}

	session, err := cfg.CreateSession()
	if err != nil {
		return nil, err
	}

	sd.session = session

	started = true
	return sd, nil
}

func (s *scylladb) ClusterConfig(keyspace string) (*gocql.ClusterConfig, error) {
	dockerIP, err := docker.DockerIP()
	if err != nil {
		return nil, err
	}

	hp, err := s.c.URL(docker.ProtoTCP, 9042)
	if err != nil {
		return nil, err
	}

	cluster := gocql.NewCluster(dockerIP)
	cluster.Port = int(hp.Port)
	cluster.ConnectTimeout = 5 * time.Second
	cluster.Timeout = 5 * time.Second

	if keyspace != "" {
		cluster.Keyspace = keyspace
	}

	return cluster, nil
}

func (s *scylladb) CreateKeyspace(name string) error {
	if err := validateKeyspaceName(name); err != nil {
		return err
	}

	q := fmt.Sprintf(
		"CREATE KEYSPACE %s WITH replication = { 'class' : 'SimpleStrategy', 'replication_factor' : 1 }",
		quoteCQLIdentifier(name),
	)
	return s.session.Query(q).Exec()
}

func (s *scylladb) DropKeyspace(name string) error {
	if err := validateKeyspaceName(name); err != nil {
		return err
	}

	q := fmt.Sprintf(
		"DROP KEYSPACE %s",
		quoteCQLIdentifier(name),
	)
	return s.session.Query(q).Exec()
}

func (s *scylladb) Close(ctx context.Context) error {
	if s.session != nil {
		s.session.Close()
	}
	return s.c.Close(ctx)
}
