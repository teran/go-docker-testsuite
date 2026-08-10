package opensearch_test

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/opensearch-project/opensearch-go/v4/opensearchapi"

	"github.com/teran/go-docker-testsuite/applications/opensearch"
)

// This example demonstrates starting an OpenSearch container, creating an
// index, indexing a document and fetching it back.
func Example() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	app, err := opensearch.NewWithImage(ctx, "index.docker.io/opensearchproject/opensearch:2.19.6")
	if err != nil {
		fmt.Printf("error: %v (is Docker running?)\n", err)
		return
	}
	defer func() { _ = app.Close(ctx) }()

	client, err := app.Client()
	if err != nil {
		fmt.Printf("error creating client: %v\n", err)
		return
	}

	api := opensearchapi.NewFromClient(client)

	if _, err := api.Indices.Create(ctx, opensearchapi.IndicesCreateReq{
		Index: "my-index",
	}); err != nil {
		fmt.Printf("error creating index: %v\n", err)
		return
	}

	if _, err := api.Document.Create(ctx, opensearchapi.DocumentCreateReq{
		Index:      "my-index",
		DocumentID: "1",
		Body:       strings.NewReader(`{"message":"Hello, World!"}`),
	}); err != nil {
		fmt.Printf("error indexing document: %v\n", err)
		return
	}

	resp, err := api.Document.Get(ctx, opensearchapi.DocumentGetReq{
		Index:      "my-index",
		DocumentID: "1",
	})
	if err != nil {
		fmt.Printf("error getting document: %v\n", err)
		return
	}

	fmt.Printf("document found: %t, source: %s\n", resp.Found, resp.Source)
}
