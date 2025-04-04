package docker

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/teran/go-random"
)

type Group interface {
	Run(ctx context.Context) error
	Close(ctx context.Context) error
}

type group struct {
	name string
	apps []*Application

	cli       *client.Client
	networkID string
}

func NewGroup(name string, apps ...*Application) (Group, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}

	return NewGroupWithClient(cli, name, apps...)
}

func NewGroupWithClient(cli *client.Client, name string, apps ...*Application) (Group, error) {
	return &group{
		name: fmt.Sprintf("%s-%s", name, random.String(random.AlphaNumeric, 14)),
		apps: apps,
		cli:  cli,
	}, nil
}

func (g *group) Close(ctx context.Context) error {
	for i := len(g.apps) - 1; i >= 0; i-- {
		app := g.apps[i]

		err := runHooks(ctx, app, HookTypeBeforeClose)
		if err != nil {
			return err
		}

		err = app.container.Close(ctx)
		if err != nil {
			return errors.Wrapf(err, "error closing container %s: `%s`", app.container.Name(), HookTypeBeforeClose)
		}

		err = runHooks(ctx, app, HookTypeAfterClose)
		if err != nil {
			return err
		}
	}

	err := g.cli.NetworkRemove(ctx, g.networkID)
	if err != nil {
		return errors.Wrapf(err, "error removing network `%s`", g.networkID)
	}
	return nil
}

func (g *group) Run(ctx context.Context) error {
	log.WithFields(log.Fields{
		"name": g.name,
	}).Trace("creating network")

	net, err := g.cli.NetworkCreate(ctx, g.name, network.CreateOptions{
		Attachable: true,
		Internal:   true,
	})
	if err != nil {
		return err
	}

	g.networkID = net.ID

	log.WithFields(log.Fields{
		"name": g.name,
		"id":   g.networkID,
	}).Debug("network created")

	for _, app := range g.apps {
		err = app.container.NetworkAttach(g.networkID)
		if err != nil {
			return errors.Wrapf(err, "error attaching to network `%s`", g.networkID)
		}

		err = runHooks(ctx, app, HookTypeBeforeRun)
		if err != nil {
			return err
		}

		err = app.container.Run(ctx)
		if err != nil {
			return errors.Wrapf(err, "error running app `%s`", app.container.Name())
		}

		err = runHooks(ctx, app, HookTypeAfterRun)
		if err != nil {
			return err
		}
	}

	return nil
}

func runHooks(ctx context.Context, app *Application, ht HookType) error {
	if len(app.hooks) > 0 {
		for _, h := range app.hooks {
			err := h(ctx, ht, app.container)
			if err != nil {
				return errors.Wrapf(err, "error calling `%s` hook for `%s`", HookTypeBeforeClose, app.container.Name())
			}
		}
	}
	return nil
}
