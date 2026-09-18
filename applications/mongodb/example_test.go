package mongodb_test

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/teran/go-docker-testsuite/applications/mongodb"
)

// This example demonstrates starting a MongoDB 7.0 container, connecting via
// the official mongo-driver, inserting a document and reading it back.
func Example() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	app, err := mongodb.NewWithImage(ctx, "index.docker.io/library/mongo:7.0.43")
	if err != nil {
		fmt.Printf("error: %v (is Docker running?)\n", err)
		return
	}
	defer func() { _ = app.Close(ctx) }()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(app.MustURI("example")))
	if err != nil {
		fmt.Printf("error connecting: %v\n", err)
		return
	}
	defer func() { _ = client.Disconnect(ctx) }()

	coll := client.Database("example").Collection("items")
	if _, err := coll.InsertOne(ctx, bson.M{"name": "hello"}); err != nil {
		fmt.Printf("error inserting: %v\n", err)
		return
	}

	var result bson.M
	if err := coll.FindOne(ctx, bson.M{"name": "hello"}).Decode(&result); err != nil {
		fmt.Printf("error reading: %v\n", err)
		return
	}
	fmt.Printf("inserted and read back: %v\n", result["name"])
}
