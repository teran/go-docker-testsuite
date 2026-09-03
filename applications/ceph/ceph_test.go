package ceph

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/require"

	"github.com/teran/go-docker-testsuite/images"
)

const testBucketName = "test-bucket"

// imageUnderTest returns the image to run, overridable via CEPH_IMAGE so any
// published ghcr.io/teran/ceph-container/ceph:v<version> image (squid or
// tentacle) can be exercised without editing the default.
func imageUnderTest() string {
	if v := os.Getenv("CEPH_IMAGE"); v != "" {
		return v
	}
	return images.Ceph
}

func TestCeph(t *testing.T) {
	r := require.New(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	app, err := NewWithImage(ctx, imageUnderTest())
	r.NoError(err)
	defer func() { _ = app.Close(ctx) }()

	cli, err := app.Client()
	r.NoError(err)
	r.NotNil(cli)

	r.Equal(DefaultAccessKey, app.AccessKey())
	r.Equal(DefaultSecretKey, app.SecretKey())

	err = cli.MakeBucket(ctx, testBucketName, minio.MakeBucketOptions{})
	r.NoError(err)

	testPayload := "test data"
	_, err = cli.PutObject(
		ctx,
		testBucketName,
		"some_key",
		strings.NewReader(testPayload),
		int64(len(testPayload)),
		minio.PutObjectOptions{},
	)
	r.NoError(err)

	obj, err := cli.GetObject(ctx, testBucketName, "some_key", minio.GetObjectOptions{})
	r.NoError(err)

	resp, err := io.ReadAll(obj)
	r.NoError(err)
	r.Equal(testPayload, string(resp))
}
