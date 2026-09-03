package ceph_test

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"

	"github.com/teran/go-docker-testsuite/applications/ceph"
)

// This example demonstrates starting a Ceph RGW container, creating a bucket,
// and uploading/downloading an object using the S3 (minio-go) client.
func Example() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	app, err := ceph.New(ctx)
	if err != nil {
		fmt.Printf("error: %v (is Docker running?)\n", err)
		return
	}
	defer func() { _ = app.Close(ctx) }()

	endpoint, err := app.Endpoint()
	if err != nil {
		fmt.Printf("error getting endpoint: %v\n", err)
		return
	}
	fmt.Println("ceph started:", endpoint)

	cli, err := app.Client()
	if err != nil {
		fmt.Printf("error creating client: %v\n", err)
		return
	}

	if err := cli.MakeBucket(ctx, "example", minio.MakeBucketOptions{}); err != nil {
		fmt.Printf("error creating bucket: %v\n", err)
		return
	}
	fmt.Println("bucket created")

	payload := "Hello, Ceph!"
	_, err = cli.PutObject(ctx, "example", "hello.txt",
		strings.NewReader(payload), int64(len(payload)),
		minio.PutObjectOptions{ContentType: "text/plain"},
	)
	if err != nil {
		fmt.Printf("error uploading: %v\n", err)
		return
	}
	fmt.Println("object uploaded")

	obj, err := cli.GetObject(ctx, "example", "hello.txt", minio.GetObjectOptions{})
	if err != nil {
		fmt.Printf("error downloading: %v\n", err)
		return
	}
	defer func() { _ = obj.Close() }()

	data, _ := io.ReadAll(obj)
	fmt.Printf("object content: %s\n", string(data))
}
