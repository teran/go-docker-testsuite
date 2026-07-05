package postgres_test

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v4"

	"github.com/teran/go-docker-testsuite/applications/postgres"
)

// This example demonstrates starting a PostgreSQL container, creating a
// database, connecting via pgx, executing queries, and cleaning up.
func Example() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	app, err := postgres.New(ctx)
	if err != nil {
		fmt.Printf("error: %v (is Docker running?)\n", err)
		return
	}
	defer func() { _ = app.Close(ctx) }()

	if err := app.CreateDB(ctx, "example_db"); err != nil {
		fmt.Printf("error creating database: %v\n", err)
		return
	}
	fmt.Println("database created")

	conn, err := pgx.Connect(ctx, app.MustDSN("example_db"))
	if err != nil {
		fmt.Printf("error connecting: %v\n", err)
		return
	}
	defer func() { _ = conn.Close(ctx) }()

	var result int
	if err := conn.QueryRow(ctx, "SELECT 42").Scan(&result); err != nil {
		fmt.Printf("error querying: %v\n", err)
		return
	}
	fmt.Printf("query returned %d\n", result)

	if err := app.DropDB(ctx, "example_db"); err != nil {
		fmt.Printf("error dropping database: %v\n", err)
		return
	}
	fmt.Println("database dropped")
}
