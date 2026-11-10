package tmt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

var topicPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type NewOptions struct {
	Base   string
	Topic  string
	Branch string
	Repos  []string
}

func (a *App) New(ctx context.Context, o NewOptions) error {
	t, err := a.create(ctx, o)
	if err != nil {
		return err
	}
	return a.Mux.Attach(ctx, t.Session)
}

func (a *App) create(ctx context.Context, o NewOptions) (t Task, err error) {
	base, err := a.resolveBase(ctx, o.Base)
	if err != nil {
		return Task{}, err
	}
	if !topicPattern.MatchString(o.Topic) {
		return Task{}, fmt.Errorf("topic must match %s", topicPattern)
	}
	if err := a.checkRepos(base, o.Repos); err != nil {
		return Task{}, err
	}
	repos := slices.Compact(slices.Sorted(slices.Values(o.Repos)))

	baseName := filepath.Base(base)
	name := baseName + "-" + o.Topic
	branch := o.Branch
	if branch == "" {
		branch = a.Config.BranchPrefix + o.Topic + "-1"
	}
	t = Task{
		Dir:     filepath.Join(a.Config.Root, baseName, name),
		Topic:   name,
		Base:    base,
		Branch:  branch,
		Session: name,
	}
	if _, err := os.Lstat(t.Dir); err == nil {
		return Task{}, fmt.Errorf("%s already exists", t.Dir)
	}

	if err := a.Tasks.Create(ctx, t); err != nil {
		return Task{}, err
	}
	defer func() {
		if err != nil {
			a.rollback(context.WithoutCancel(ctx), t, nil)
		}
	}()

	for _, r := range repos {
		if err := os.Mkdir(filepath.Join(t.Dir, r), 0o755); err != nil {
			return Task{}, err
		}
	}
	if _, err := a.Tasks.LinkMissing(ctx, t); err != nil {
		return Task{}, err
	}
	if err := a.ensureSession(ctx, t, a.fillCommand(t, repos)); err != nil {
		return Task{}, err
	}
	return t, nil
}

func (a *App) fillCommand(t Task, repos []string) string {
	args := append([]string{a.Config.Self, "_fill", t.Topic}, repos...)
	for i, s := range args {
		args[i] = shellQuote(s)
	}
	return strings.Join(args, " ")
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func (a *App) Fill(ctx context.Context, topic string, repos []string) (string, error) {
	t, err := a.Tasks.ByTopic(ctx, topic)
	if err != nil {
		return "", err
	}
	a.setStatus(ctx, MuxWorking, "creating worktrees")
	if err := a.fill(ctx, t, repos); err != nil {
		a.setStatus(context.WithoutCancel(ctx), MuxBlocked, err.Error())
		return "", err
	}
	a.setStatus(ctx, MuxReady, "")
	return a.agentCommand(t), nil
}

func (a *App) fill(ctx context.Context, t Task, repos []string) (err error) {
	var created []createdBranch
	defer func() {
		if err != nil {
			a.rollback(context.WithoutCancel(ctx), t, created)
		}
	}()
	if err := a.Fresh(ctx, t.Base); err != nil {
		a.warnf("fresh failed for %s, continuing", t.Base)
	}
	for _, r := range repos {
		a.setStatus(ctx, MuxWorking, "creating worktree "+r)
		if err := a.addRepo(ctx, t, r, &created); err != nil {
			return err
		}
	}
	return a.writeContext(ctx, t)
}

func (a *App) rollback(ctx context.Context, t Task, created []createdBranch) {
	a.warnf("rolling back %s", t.Dir)
	wts, err := a.worktrees(ctx, t)
	if err != nil {
		a.warnf("%v", err)
	}
	for _, r := range wts {
		src, err := a.Git.Open(filepath.Join(t.Base, r))
		if err == nil {
			err = a.discardWorktree(ctx, src, filepath.Join(t.Dir, r), true)
		}
		if err != nil {
			a.warnf("%v", err)
		}
	}
	if err := a.Trash.Empty(); err != nil {
		a.warnf("%v", err)
	}
	for _, c := range created {
		if err := c.repo.DeleteBranch(ctx, c.branch); err != nil {
			a.warnf("%v", err)
		}
	}
	if wts, err := a.worktrees(ctx, t); err == nil && len(wts) == 0 {
		if err := a.Tasks.Remove(ctx, t); err != nil {
			a.warnf("%v", err)
		}
	}
}

func (a *App) Add(ctx context.Context, topic string, repos []string) error {
	t, err := a.resolveTask(ctx, topic)
	if err != nil {
		return err
	}
	if err := a.checkRepos(t.Base, repos); err != nil {
		return err
	}
	for _, r := range repos {
		if err := a.addRepo(ctx, t, r, nil); err != nil {
			return err
		}
	}
	return a.writeContext(ctx, t)
}

func (a *App) Open(ctx context.Context, topic string) error {
	t, err := a.resolveTask(ctx, topic)
	if err != nil {
		return err
	}
	return a.open(ctx, t)
}
