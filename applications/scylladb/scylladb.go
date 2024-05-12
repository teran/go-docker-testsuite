package scylladb

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/gocql/gocql"

	"github.com/teran/go-docker-testsuite"
	"github.com/teran/go-docker-testsuite/images"
)

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
			[]string{"--overprovisioned=1"},
			docker.NewEnvironment(),
			docker.NewPortBindings().
				PortDNAT(docker.ProtoTCP, 9042),
		)
	if err != nil {
		return nil, err
	}

	err = c.Run(ctx)
	if err != nil {
		return nil, err
	}

	ipRe := `\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\`
	l := fmt.Sprintf(
		`\[shard \d{1}\] cql_server_controller - Starting listening for CQL clients on %s:9042 \(unencrypted, non-shard-aware\)$`,
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
	q := fmt.Sprintf(
		"CREATE KEYSPACE %s WITH replication = { 'class' : 'SimpleStrategy', 'replication_factor' : 1 }",
		name,
	)
	return s.session.Query(q).Exec()
}

func (s *scylladb) DropKeyspace(name string) error {
	q := fmt.Sprintf(
		"DROP KEYSPACE %s",
		name,
	)
	return s.session.Query(q).Exec()
}

func (s *scylladb) Close(ctx context.Context) error {
	return s.c.Close(ctx)
}
