package versions

import (
	"context"
	"strings"

	"github.com/opensearch-project/opensearch-go/v4/opensearchapi"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/suite"

	"github.com/teran/go-docker-testsuite/applications/opensearch"
)

func init() {
	log.SetLevel(log.TraceLevel)
}

type OpenSearchTestSuite struct {
	suite.Suite

	ctx   context.Context
	image string

	app opensearch.OpenSearch
}

func New(ctx context.Context, image string) *OpenSearchTestSuite {
	return &OpenSearchTestSuite{
		ctx:   ctx,
		image: image,
	}
}

func (s *OpenSearchTestSuite) TestOpenSearch() {
	client, err := s.app.Client()
	s.Require().NoError(err)

	api := opensearchapi.NewFromClient(client)

	_, err = api.Indices.Create(s.ctx, opensearchapi.IndicesCreateReq{
		Index: "test-index",
	})
	s.Require().NoError(err)

	_, err = api.Document.Create(s.ctx, opensearchapi.DocumentCreateReq{
		Index:      "test-index",
		DocumentID: "1",
		Body:       strings.NewReader(`{"message":"hello"}`),
	})
	s.Require().NoError(err)

	resp, err := api.Document.Get(s.ctx, opensearchapi.DocumentGetReq{
		Index:      "test-index",
		DocumentID: "1",
	})
	s.Require().NoError(err)
	s.Require().True(resp.Found)
	s.Require().Equal(`{"message":"hello"}`, string(resp.Source))
}

func (s *OpenSearchTestSuite) SetupTest() {
	var err error
	s.app, err = opensearch.NewWithImage(s.ctx, s.image)
	s.Require().NoError(err)
}

func (s *OpenSearchTestSuite) TearDownTest() {
	if s.app == nil {
		return
	}
	err := s.app.Close(s.ctx)
	s.Require().NoError(err)
}
