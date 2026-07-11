package rabbitmq

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	docker "github.com/teran/go-docker-testsuite"
	"github.com/teran/go-docker-testsuite/images"
)

const (
	amqpPort       = 5672
	managementPort = 15672

	defaultUser     = "guest"
	defaultPassword = "guest"
)

type RabbitMQ interface {
	Close(ctx context.Context) error

	GetAMQPURL(ctx context.Context) (string, error)
	GetManagementURL(ctx context.Context) (string, error)

	CreateVHost(ctx context.Context, name string) error
	CreateUser(ctx context.Context, username, password string) error
	SetPermissions(ctx context.Context, vhost, username, configure, write, read string) error
}

type rabbitmq struct {
	c docker.Container
}

func New(ctx context.Context) (RabbitMQ, error) {
	return NewWithImage(ctx, images.RabbitMQ)
}

func NewWithImage(ctx context.Context, image string) (RabbitMQ, error) {
	c, err := docker.NewContainer(
		"rabbitmq",
		image,
		nil,
		docker.NewEnvironment().
			StringVar("RABBITMQ_DEFAULT_USER", defaultUser).
			StringVar("RABBITMQ_DEFAULT_PASS", defaultPassword),
		docker.NewPortBindings().
			PortDNAT(docker.ProtoTCP, amqpPort).
			PortDNAT(docker.ProtoTCP, managementPort),
	)
	if err != nil {
		return nil, err
	}

	if err := c.Run(ctx); err != nil {
		return nil, err
	}

	if err := c.AwaitOutput(ctx, docker.NewSubstringMatcher("Server startup complete")); err != nil {
		return nil, err
	}

	time.Sleep(1 * time.Second)

	return &rabbitmq{
		c: c,
	}, nil
}

func (r *rabbitmq) Close(ctx context.Context) error {
	return r.c.Close(ctx)
}

func (r *rabbitmq) GetAMQPURL(ctx context.Context) (string, error) {
	hp, err := r.c.URL(docker.ProtoTCP, amqpPort)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("amqp://guest:guest@%s/", hp.String()), nil
}

func (r *rabbitmq) GetManagementURL(ctx context.Context) (string, error) {
	hp, err := r.c.URL(docker.ProtoTCP, managementPort)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("http://%s", hp.String()), nil
}

func (r *rabbitmq) CreateVHost(ctx context.Context, name string) error {
	mgmtURL, err := r.GetManagementURL(ctx)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut,
		fmt.Sprintf("%s/api/vhosts/%s", mgmtURL, url.PathEscape(name)), nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(defaultUser, defaultPassword)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected response creating vhost %q: %d %s", name, resp.StatusCode, string(body))
	}

	return nil
}

func (r *rabbitmq) CreateUser(ctx context.Context, username, password string) error {
	mgmtURL, err := r.GetManagementURL(ctx)
	if err != nil {
		return err
	}

	body := fmt.Sprintf(`{"password":"%s","tags":"administrator"}`, password)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut,
		fmt.Sprintf("%s/api/users/%s", mgmtURL, username),
		strings.NewReader(body))
	if err != nil {
		return err
	}
	req.SetBasicAuth(defaultUser, defaultPassword)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected response creating user %q: %d %s", username, resp.StatusCode, string(body))
	}

	return nil
}

func (r *rabbitmq) SetPermissions(ctx context.Context, vhost, username, configure, write, read string) error {
	mgmtURL, err := r.GetManagementURL(ctx)
	if err != nil {
		return err
	}

	body := fmt.Sprintf(
		`{"configure":"%s","write":"%s","read":"%s"}`,
		configure, write, read,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut,
		fmt.Sprintf("%s/api/permissions/%s/%s", mgmtURL, url.PathEscape(vhost), username),
		strings.NewReader(body))
	if err != nil {
		return err
	}
	req.SetBasicAuth(defaultUser, defaultPassword)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected response setting permissions for %q on %q: %d %s",
			username, vhost, resp.StatusCode, string(body))
	}

	return nil
}
