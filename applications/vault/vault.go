package vault

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/hashicorp/vault-client-go"
	"github.com/hashicorp/vault-client-go/schema"
	log "github.com/sirupsen/logrus"
	docker "github.com/teran/go-docker-testsuite"
)

var reTokenMatch = regexp.MustCompile(`Root Token: (.+)$`)

type Vault interface {
	ClusterAddr() (string, error)
	APIAddr() (string, error)
	GetRootToken(ctx context.Context) (string, error)
	GetRootClient(ctx context.Context) (*vault.Client, error)
	CreateEngine(ctx context.Context, mountPath, engineType string) error
	RemoveEngine(ctx context.Context, mountPath string) error
	Close(ctx context.Context) error
}

type vaultImpl struct {
	c docker.Container
}

func New(ctx context.Context, image string) (Vault, error) {
	c, err := docker.NewContainer(
		"vault",
		image,
		nil,
		docker.NewEnvironment().
			StringVar("VAULT_LOG_LEVEL", "trace"),
		docker.NewPortBindings().
			PortDNAT(docker.ProtoTCP, 8200).
			PortDNAT(docker.ProtoTCP, 8201),
	)
	if err != nil {
		return nil, err
	}

	err = c.Run(ctx)
	if err != nil {
		return nil, err
	}

	err = c.AwaitOutput(ctx, docker.NewRegexpMatcher(reTokenMatch))
	if err != nil {
		return nil, err
	}

	return &vaultImpl{
		c: c,
	}, nil
}

func (v *vaultImpl) ClusterAddr() (string, error) {
	u, err := v.c.URL(docker.ProtoTCP, 8200)
	if err != nil {
		return "", err
	}

	log.Trace("Vault address: " + u.String())

	return u.String(), nil
}

func (v *vaultImpl) APIAddr() (string, error) {
	u, err := v.c.URL(docker.ProtoTCP, 8201)
	if err != nil {
		return "", err
	}

	log.Trace("Vault address: " + u.String())

	return u.String(), nil
}

func (v *vaultImpl) GetRootToken(ctx context.Context) (string, error) {
	lines, err := v.c.GetOutput(ctx, docker.NewRegexpMatcher(reTokenMatch))
	if err != nil {
		return "", err
	}

	if len(lines) != 1 {
		return "", errors.New("unexpected number of root token lines")
	}

	parts := strings.SplitN(lines[0], ": ", 2)
	if len(parts) != 2 {
		return "", errors.New("unexpected root token line format")
	}
	return strings.TrimSpace(parts[1]), nil
}

func (v *vaultImpl) GetRootClient(ctx context.Context) (*vault.Client, error) {
	cli, err := v.cli(ctx)
	if err != nil {
		return nil, err
	}

	return cli, nil
}

func (v *vaultImpl) Close(ctx context.Context) error {
	return v.c.Close(ctx)
}

func (v *vaultImpl) CreateEngine(ctx context.Context, mountPath, engineType string) error {
	cli, err := v.cli(ctx)
	if err != nil {
		return err
	}

	_, err = cli.System.MountsEnableSecretsEngine(
		ctx, mountPath, schema.MountsEnableSecretsEngineRequest{
			Type:    engineType,
			Options: map[string]any{},
		},
	)
	if err != nil {
		return err
	}

	return nil
}

func (v *vaultImpl) RemoveEngine(ctx context.Context, mountPath string) error {
	cli, err := v.cli(ctx)
	if err != nil {
		return err
	}

	_, err = cli.System.MountsDisableSecretsEngine(ctx, mountPath)
	if err != nil {
		return err
	}

	return nil
}

func (v *vaultImpl) cli(ctx context.Context) (*vault.Client, error) {
	hp, err := v.ClusterAddr()
	if err != nil {
		return nil, err
	}

	cli, err := vault.New(
		vault.WithAddress(fmt.Sprintf("http://%s", hp)),
		vault.WithRequestTimeout(30*time.Second),
	)
	if err != nil {
		return nil, err
	}

	rootToken, err := v.GetRootToken(ctx)
	if err != nil {
		return nil, err
	}

	err = cli.SetToken(rootToken)
	if err != nil {
		return nil, err
	}

	return cli, nil
}
