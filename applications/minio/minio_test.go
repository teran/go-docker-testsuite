package minio

import (
	"context"
	"io/ioutil"
	"strings"
	"testing"
	"time"

	minioSDK "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	log "github.com/sirupsen/logrus"

	"github.com/stretchr/testify/require"
)

const testBucketName = "test-bucket"

func init() {
	log.SetLevel(log.TraceLevel)
}

func TestMinio(t *testing.T) {
	r := require.New(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	app, err := New(ctx)
	r.NoError(err)

	defer app.Close(ctx)

	s3Endpoint, err := app.GetEndpointURL()
	r.NoError(err)
	r.NotEmpty(s3Endpoint)

	consoleEndpoint, err := app.GetConsoleURL()
	r.NoError(err)
	r.NotEmpty(consoleEndpoint)

	cli, err := minioSDK.New(s3Endpoint, &minioSDK.Options{
		Creds:  credentials.NewStaticV4(MinioAccessKey, MinioAccessKeySecret, ""),
		Secure: false,
	})
	r.NoError(err)
	r.NotNil(cli)

	err = cli.MakeBucket(ctx, testBucketName, minioSDK.MakeBucketOptions{})
	r.NoError(err)

	testPayload := "test data"
	_, err = cli.PutObject(
		ctx,
		testBucketName,
		"some_key",
		strings.NewReader(testPayload),
		int64(len(testPayload)),
		minioSDK.PutObjectOptions{},
	)
	r.NoError(err)

	obj, err := cli.GetObject(
		ctx,
		testBucketName,
		"some_key",
		minioSDK.GetObjectOptions{},
	)
	r.NoError(err)

	resp, err := ioutil.ReadAll(obj)
	r.NoError(err)
	r.Equal(testPayload, string(resp))
}
