package tmt

import "context"

type Agent interface {
	Command(dir string) string
	Trust(ctx context.Context, dir string) error
	WriteContext(ctx context.Context, t Task, worktrees []string) error
	ReadContext(ctx context.Context, t Task) (string, error)
}

type EnvTrust interface {
	Allow(ctx context.Context, dir string) error
	Wrap(dir, command string) string
}

type DevShell interface {
	Write(ctx context.Context, t Task) error
}
