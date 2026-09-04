package ceph_test

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/teran/go-docker-testsuite/applications/ceph"
)

// This example demonstrates starting a Ceph RGW container, creating a bucket,
// and uploading/downloading an object using the AWS SDK v2 S3 client.
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

	if _, err := cli.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String("example")}); err != nil {
		fmt.Printf("error creating bucket: %v\n", err)
		return
	}
	fmt.Println("bucket created")

	payload := "Hello, Ceph!"
	_, err = cli.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String("example"),
		Key:    aws.String("hello.txt"),
		Body:   strings.NewReader(payload),
	})
	if err != nil {
		fmt.Printf("error uploading: %v\n", err)
		return
	}
	fmt.Println("object uploaded")

	obj, err := cli.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String("example"),
		Key:    aws.String("hello.txt"),
	})
	if err != nil {
		fmt.Printf("error downloading: %v\n", err)
		return
	}
	defer func() { _ = obj.Body.Close() }()

	data, _ := io.ReadAll(obj.Body)
	fmt.Printf("object content: %s\n", string(data))
}
