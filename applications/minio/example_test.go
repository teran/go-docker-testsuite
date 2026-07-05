package minio_test

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	minioSDK "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/teran/go-docker-testsuite/applications/minio"
)

// This example demonstrates starting a MinIO container, creating a bucket,
// uploading and downloading objects using the MinIO Go SDK.
func Example() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	app, err := minio.New(ctx)
	if err != nil {
		fmt.Printf("error: %v (is Docker running?)\n", err)
		return
	}
	defer app.Close(ctx)

	endpoint, err := app.GetEndpointURL()
	if err != nil {
		fmt.Printf("error getting endpoint: %v\n", err)
		return
	}
	fmt.Println("minio started:", endpoint)

	cli, err := minioSDK.New(endpoint, &minioSDK.Options{
		Creds:  credentials.NewStaticV4(minio.MinioAccessKey, minio.MinioAccessKeySecret, ""),
		Secure: false,
	})
	if err != nil {
		fmt.Printf("error creating client: %v\n", err)
		return
	}

	if err := cli.MakeBucket(ctx, "example", minioSDK.MakeBucketOptions{}); err != nil {
		fmt.Printf("error creating bucket: %v\n", err)
		return
	}
	fmt.Println("bucket created")

	payload := "Hello, MinIO!"
	_, err = cli.PutObject(ctx, "example", "hello.txt",
		strings.NewReader(payload), int64(len(payload)),
		minioSDK.PutObjectOptions{ContentType: "text/plain"},
	)
	if err != nil {
		fmt.Printf("error uploading: %v\n", err)
		return
	}
	fmt.Println("object uploaded")

	obj, err := cli.GetObject(ctx, "example", "hello.txt", minioSDK.GetObjectOptions{})
	if err != nil {
		fmt.Printf("error downloading: %v\n", err)
		return
	}
	defer obj.Close()

	data, _ := io.ReadAll(obj)
	fmt.Printf("object content: %s\n", string(data))
}
