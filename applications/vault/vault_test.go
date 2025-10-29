package vault

import (
	"fmt"
	"testing"
	"time"

	vault "github.com/hashicorp/vault-client-go"
	"github.com/hashicorp/vault-client-go/schema"
	"github.com/stretchr/testify/suite"
	"github.com/teran/go-collection/random"
)

const (
	image = "index.docker.io/hashicorp/vault:1.21.0"
)

func (s *vaultTestSuite) TestSomething() {
	_, err := s.cli().Secrets.KvV2Write(s.T().Context(), s.engineName, schema.KvV2WriteRequest{
		Data: map[string]any{
			"foo": "bar",
		},
	}, vault.WithMountPath(s.engineName))
	s.Require().NoError(err)

	sec, err := s.cli().Secrets.KvV2Read(s.T().Context(), s.engineName, vault.WithMountPath(s.engineName))
	s.Require().NoError(err)
	s.Require().Equal("bar", sec.Data.Data["foo"])
}

// ========================================================================
// Test suite setup
// ========================================================================
type vaultTestSuite struct {
	suite.Suite

	app        Vault
	engineName string
	rootToken  string
}

func (s *vaultTestSuite) SetupSuite() {
	var err error
	s.app, err = New(s.T().Context(), image)
	s.Require().NoError(err)

	s.rootToken, err = s.app.GetRootToken(s.T().Context())
	s.Require().NoError(err)
}

func (s *vaultTestSuite) cli() *vault.Client {
	hp, err := s.app.ClusterAddr()
	s.Require().NoError(err)

	cli, err := vault.New(
		vault.WithAddress(fmt.Sprintf("http://%s", hp)),
		vault.WithRequestTimeout(30*time.Second),
	)
	s.Require().NoError(err)

	err = cli.SetToken(s.rootToken)
	s.Require().NoError(err)

	return cli
}

func (s *vaultTestSuite) SetupTest() {
	s.engineName = random.String(append(random.AlphaLower, random.Numeric...), 8)

	_, err := s.cli().System.MountsEnableSecretsEngine(
		s.T().Context(), s.engineName, schema.MountsEnableSecretsEngineRequest{
			Type:    "kv-v2",
			Options: map[string]any{},
		},
	)
	s.Require().NoError(err)
}

func (s *vaultTestSuite) TearDownTest() {
	_, err := s.cli().System.MountsDisableSecretsEngine(
		s.T().Context(), s.engineName,
	)
	s.Require().NoError(err)
}

func (s *vaultTestSuite) TearDownSuite() {
	err := s.app.Close(s.T().Context())
	s.Require().NoError(err)
}

func TestVaultTestSuite(t *testing.T) {
	suite.Run(t, &vaultTestSuite{})
}
