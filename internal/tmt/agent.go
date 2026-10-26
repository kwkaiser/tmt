package tmt

import "context"

type Agent interface {
	Command() string
	Trust(ctx context.Context, dir string) error
	WriteContext(ctx context.Context, t Task, worktrees []string) error
}

type EnvTrust interface {
	Allow(ctx context.Context, dir string) error
}
