package tmt

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
)

type removeOptions struct {
	force       bool
	yes         bool
	promptForce bool
}

func (a *App) removeTask(ctx context.Context, t Task, o removeOptions) error {
	wts, err := a.worktrees(ctx, t)
	if err != nil {
		return err
	}

	states := make([]repoState, len(wts))
	errs := make([]error, len(wts))
	var wg sync.WaitGroup
	for i, r := range wts {
		wg.Go(func() { states[i], errs[i] = a.repoStatus(ctx, t, r) })
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return err
	}

	needPR := make([]bool, len(wts))
	var refs []branchRef
	for i, s := range states {
		if (s.Own > 0 || s.Unpushed > 0) && (s.Dirty == 0 || o.force || o.promptForce) {
			needPR[i] = true
			refs = append(refs, branchRef{Src: filepath.Join(t.Base, wts[i]), Branch: s.Branch})
		}
	}
	prs := a.newPRCache()
	prs.prefetch(ctx, refs)

	blocked := false
	for i, r := range wts {
		if needPR[i] {
			states[i].PR = prs.state(ctx, filepath.Join(t.Base, r), states[i].Branch)
		}
		if s := states[i]; s.Orphaned {
			a.warnf("%s is orphaned from %s, cannot check for changes", r, filepath.Join(t.Base, r))
			blocked = true
		} else if !s.safe() {
			a.warnf("%s has dirty:%d unpushed:%d pr:%s", r, s.Dirty, s.Unpushed, s.PR)
			blocked = true
		}
	}
	if blocked && !o.force {
		if !o.promptForce {
			return fmt.Errorf("refusing to remove %s; %w", t.Topic, ErrUnsafe)
		}
		ok, err := a.Prompt.Confirm(ctx, fmt.Sprintf("force remove task %s?", t.Topic))
		if err != nil || !ok {
			return err
		}
		o.force, o.yes = true, true
	}

	if !o.yes {
		ok, err := a.Prompt.Confirm(ctx, fmt.Sprintf("Remove task %s (%s)?", t.Topic, strings.Join(wts, " ")))
		if err != nil {
			return err
		}
		if !ok {
			return ErrAborted
		}
	}

	for i, r := range wts {
		if states[i].Orphaned {
			if err := a.Trash.Discard(filepath.Join(t.Dir, r)); err != nil {
				return err
			}
			a.logf("removed orphaned %s worktree", r)
			continue
		}
		src, err := a.Git.Open(filepath.Join(t.Base, r))
		if err != nil {
			return err
		}
		if err := a.discardWorktree(ctx, src, filepath.Join(t.Dir, r), o.force); err != nil {
			return err
		}
		s := states[i]
		if s.Own == 0 || s.PR.Done() {
			if err := src.DeleteBranch(ctx, s.Branch); err != nil {
				return err
			}
			a.logf("removed %s worktree and branch %s", r, s.Branch)
		} else {
			a.logf("removed %s worktree, kept branch %s", r, s.Branch)
		}
	}

	if err := a.Trash.Empty(); err != nil {
		return err
	}
	if left, err := a.worktrees(ctx, t); err != nil {
		return err
	} else if len(left) > 0 {
		return fmt.Errorf("worktrees remain in %s, not deleting it", t.Dir)
	}
	if err := a.Tasks.Remove(ctx, t); err != nil {
		return err
	}
	return a.Mux.KillSession(ctx, t.Session)
}

type DoneOptions struct {
	Topic string
	Force bool
	Yes   bool
}

func (a *App) Done(ctx context.Context, o DoneOptions) error {
	t, err := a.resolveTask(ctx, o.Topic)
	if err != nil {
		return err
	}
	return a.removeTask(ctx, t, removeOptions{force: o.Force, yes: true, promptForce: !o.Yes})
}

func (a *App) Close(ctx context.Context) error {
	session, err := a.Mux.CurrentSession(ctx)
	if err != nil {
		return err
	}
	if session == "" {
		return errors.New("not inside a mux session")
	}
	t, err := a.Tasks.BySession(ctx, session)
	if errors.Is(err, ErrTaskNotFound) {
		ok, err := a.Prompt.Confirm(ctx, fmt.Sprintf("close %s?", session))
		if err != nil || !ok {
			return err
		}
		return a.Mux.KillSession(ctx, session)
	}
	if err != nil {
		return err
	}
	return a.removeTask(ctx, t, removeOptions{promptForce: true})
}
