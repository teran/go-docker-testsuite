package versions

import (
	"context"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/teran/go-docker-testsuite/applications/mongodb"
)

func init() {
	log.SetLevel(log.TraceLevel)
}

type MongoTestSuite struct {
	suite.Suite

	ctx   context.Context
	image string

	app mongodb.Mongo
}

func New(ctx context.Context, image string) *MongoTestSuite {
	return &MongoTestSuite{
		ctx:   ctx,
		image: image,
	}
}

func (s *MongoTestSuite) TestMongoDB() {
	client, err := mongo.Connect(s.ctx, options.Client().ApplyURI(s.app.MustURI("testdb")))
	s.Require().NoError(err)
	defer func() { _ = client.Disconnect(s.ctx) }()

	coll := client.Database("testdb").Collection("testcoll")

	_, err = coll.InsertOne(s.ctx, bson.M{"key": "value"})
	s.Require().NoError(err)

	var result bson.M
	err = coll.FindOne(s.ctx, bson.M{"key": "value"}).Decode(&result)
	s.Require().NoError(err)
	s.Require().Equal("value", result["key"])
}

func (s *MongoTestSuite) TestDDL() {
	client, err := mongo.Connect(s.ctx, options.Client().ApplyURI(s.app.MustURI("")))
	s.Require().NoError(err)
	defer func() { _ = client.Disconnect(s.ctx) }()

	err = s.app.CreateDatabase(s.ctx, "ddldb")
	s.Require().NoError(err)

	// Confirm the database is materialized. MongoDB creates databases lazily
	// on first write, so CreateDatabase forces a collection into existence to
	// make the database appear in the server's list.
	names, err := client.ListDatabaseNames(s.ctx, bson.M{"name": "ddldb"})
	s.Require().NoError(err)
	s.Require().Contains(names, "ddldb")

	err = s.app.DropDatabase(s.ctx, "ddldb")
	s.Require().NoError(err)

	// MongoDB's dropDatabase is idempotent: dropping a non-existent database
	// is a no-op and the driver returns nil (verified empirically on 7.0.43
	// and 8.0.4), so assert no error rather than an error.
	err = s.app.DropDatabase(s.ctx, "nonexistent")
	s.Require().NoError(err)
}

func (s *MongoTestSuite) SetupTest() {
	var err error
	s.app, err = mongodb.New(s.ctx, s.image)
	s.Require().NoError(err)
}

func (s *MongoTestSuite) TearDownTest() {
	err := s.app.Close(s.ctx)
	s.Require().NoError(err)
}
