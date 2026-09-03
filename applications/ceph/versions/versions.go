package versions

import (
	"context"
	"io"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/suite"

	"github.com/teran/go-docker-testsuite/applications/ceph"
)

type testSuite struct {
	suite.Suite

	ctx   context.Context
	image string
}

func New(ctx context.Context, image string) *testSuite {
	return &testSuite{
		ctx:   ctx,
		image: image,
	}
}

func (s *testSuite) TestAll() {
	app, err := ceph.NewWithImage(s.ctx, s.image)
	s.Require().NoError(err)

	defer func() {
		err := app.Close(s.ctx)
		s.Require().NoError(err)
	}()

	cli, err := app.Client()
	s.Require().NoError(err)
	s.Require().NotNil(cli)

	const bucket = "versioned-bucket"
	err = cli.MakeBucket(s.ctx, bucket, minio.MakeBucketOptions{})
	s.Require().NoError(err)

	payload := "ceph version test"
	_, err = cli.PutObject(
		s.ctx, bucket, "key",
		strings.NewReader(payload), int64(len(payload)),
		minio.PutObjectOptions{},
	)
	s.Require().NoError(err)

	obj, err := cli.GetObject(s.ctx, bucket, "key", minio.GetObjectOptions{})
	s.Require().NoError(err)

	data, err := io.ReadAll(obj)
	s.Require().NoError(err)
	s.Require().Equal(payload, string(data))
}
