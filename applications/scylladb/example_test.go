package scylladb_test

import (
	"context"
	"fmt"
	"time"

	"github.com/teran/go-docker-testsuite/applications/scylladb"
)

// This example demonstrates starting a ScyllaDB container, creating a
// keyspace and table, inserting and querying data.
func Example() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	app, err := scylladb.New(ctx)
	if err != nil {
		fmt.Printf("error: %v (is Docker running?)\n", err)
		return
	}
	defer func() { _ = app.Close(ctx) }()

	if err := app.CreateKeyspace("example_ks"); err != nil {
		fmt.Printf("error creating keyspace: %v\n", err)
		return
	}
	fmt.Println("keyspace created")

	cfg, err := app.ClusterConfig("example_ks")
	if err != nil {
		fmt.Printf("error getting cluster config: %v\n", err)
		return
	}

	session, err := cfg.CreateSession()
	if err != nil {
		fmt.Printf("error creating session: %v\n", err)
		return
	}
	defer session.Close()

	if err := session.Query(
		`CREATE TABLE users (id UUID PRIMARY KEY, name text, email text)`,
	).Exec(); err != nil {
		fmt.Printf("error creating table: %v\n", err)
		return
	}
	fmt.Println("table created")

	if err := session.Query(
		`INSERT INTO users (id, name, email) VALUES (uuid(), ?, ?)`,
		"Alice", "alice@example.com",
	).Exec(); err != nil {
		fmt.Printf("error inserting: %v\n", err)
		return
	}
	fmt.Println("row inserted")

	if err := app.DropKeyspace("example_ks"); err != nil {
		fmt.Printf("error dropping keyspace: %v\n", err)
		return
	}
	fmt.Println("keyspace dropped")
}
