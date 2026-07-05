package mysql_test

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/teran/go-docker-testsuite/applications/mysql"
)

// This example demonstrates starting a MySQL 8.0 container, creating a
// database, connecting via database/sql, and executing a query.
func Example() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	app, err := mysql.New(ctx, "index.docker.io/library/mysql:8.0.4")
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

	db, err := sql.Open("mysql", app.MustDSN("example_db"))
	if err != nil {
		fmt.Printf("error opening database: %v\n", err)
		return
	}
	defer func() { _ = db.Close() }()

	if _, err := db.ExecContext(ctx, "SELECT 1"); err != nil {
		fmt.Printf("error executing query: %v\n", err)
		return
	}
	fmt.Println("query succeeded")
}
