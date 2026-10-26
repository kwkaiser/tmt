package tmt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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

	if err := a.Fresh(ctx, base); err != nil {
		a.warnf("fresh failed for %s, continuing", base)
	}
	if err := a.Tasks.Create(ctx, t); err != nil {
		return Task{}, err
	}

	var created []createdBranch
	defer func() {
		if err != nil {
			a.rollback(context.WithoutCancel(ctx), t, created)
		}
	}()

	if _, err := a.Tasks.LinkMissing(ctx, t); err != nil {
		return Task{}, err
	}
	for _, r := range o.Repos {
		if err := a.addRepo(ctx, t, r, &created); err != nil {
			return Task{}, err
		}
	}
	if err := a.writeContext(ctx, t); err != nil {
		return Task{}, err
	}
	if err := a.ensureSession(ctx, t); err != nil {
		return Task{}, err
	}
	return t, nil
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
