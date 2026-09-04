package versions

import (
	"context"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
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
	_, err = cli.CreateBucket(s.ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)})
	s.Require().NoError(err)

	payload := "ceph version test"
	_, err = cli.PutObject(s.ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("key"),
		Body:   strings.NewReader(payload),
	})
	s.Require().NoError(err)

	obj, err := cli.GetObject(s.ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("key"),
	})
	s.Require().NoError(err)
	defer func() { _ = obj.Body.Close() }()

	data, err := io.ReadAll(obj.Body)
	s.Require().NoError(err)
	s.Require().Equal(payload, string(data))
}
