package versions

import (
	"context"

	log "github.com/sirupsen/logrus"
	"github.com/teran/go-docker-testsuite/applications/redis"

	redisClient "github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/suite"
)

func init() {
	log.SetLevel(log.TraceLevel)
}

type RedisTestSuite struct {
	suite.Suite

	ctx   context.Context
	image string

	app redis.Redis
}

func New(ctx context.Context, image string) *RedisTestSuite {
	return &RedisTestSuite{
		ctx:   ctx,
		image: image,
	}
}

func (s *RedisTestSuite) TestRedis() {
	rdb := redisClient.NewClient(&redisClient.Options{
		Addr:     s.app.MustAddr(),
		Password: "",
		DB:       0,
	})

	err := rdb.Set(s.ctx, "key", "value", 0).Err()
	s.Require().NoError(err)

	val, err := rdb.Get(s.ctx, "key").Result()
	s.Require().NoError(err)
	s.Require().Equal("value", val)
}

func (s *RedisTestSuite) SetupTest() {
	var err error
	s.app, err = redis.New(s.ctx, s.image)
	s.Require().NoError(err)
}

func (s *RedisTestSuite) TearDownTest() {
	s.app.Close(s.ctx)
}
