package vault

import (
	"context"
	"errors"
	"regexp"
	"strings"

	log "github.com/sirupsen/logrus"
	docker "github.com/teran/go-docker-testsuite"
)

var reTokenMatch = regexp.MustCompile(`Root Token: (.+)$`)

type Vault interface {
	ClusterAddr() (string, error)
	APIAddr() (string, error)
	GetRootToken(ctx context.Context) (string, error)
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

func (v *vaultImpl) Close(ctx context.Context) error {
	return v.c.Close(ctx)
}
