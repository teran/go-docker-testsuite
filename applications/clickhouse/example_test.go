package clickhouse_test

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2" // registers the "clickhouse" driver

	"github.com/teran/go-docker-testsuite/applications/clickhouse"
)

// This example demonstrates starting a ClickHouse container, creating a
// database, connecting via database/sql, executing a query, and cleaning up.
func Example() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	app, err := clickhouse.New(ctx)
	if err != nil {
		fmt.Printf("error: %v (is Docker running?)\n", err)
		return
	}
	defer func() { _ = app.Close(ctx) }()

	if err := app.CreateDatabase(ctx, "example_db"); err != nil {
		fmt.Printf("error creating database: %v\n", err)
		return
	}
	fmt.Println("database created")

	db, err := sql.Open("clickhouse", app.MustDSN("example_db"))
	if err != nil {
		fmt.Printf("error connecting: %v\n", err)
		return
	}
	defer func() { _ = db.Close() }()

	if err := db.PingContext(ctx); err != nil {
		fmt.Printf("error pinging: %v\n", err)
		return
	}

	var result int
	if err := db.QueryRowContext(ctx, "SELECT 42").Scan(&result); err != nil {
		fmt.Printf("error querying: %v\n", err)
		return
	}
	fmt.Printf("query returned %d\n", result)

	if err := app.DropDatabase(ctx, "example_db"); err != nil {
		fmt.Printf("error dropping database: %v\n", err)
		return
	}
	fmt.Println("database dropped")
}
