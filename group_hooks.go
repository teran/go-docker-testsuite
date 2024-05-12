package docker

import "context"

type HookType string

const (
	HookTypeBeforeRun   HookType = "before_run"
	HookTypeAfterRun    HookType = "after_run"
	HookTypeBeforeClose HookType = "before_close"
	HookTypeAfterClose  HookType = "after_close"
)

type Hook func(context.Context, HookType, Container) error
