package tmt

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type retiree struct {
	task   Task
	repo   string
	branch string
}

func (a *App) GC(ctx context.Context) error {
	refs, err := a.taskBranches(ctx)
	if err != nil {
		return err
	}
	prs := a.newPRCache()
	prs.prefetch(ctx, refs)

	var srcs []string
	seen := map[string]bool{}
	for _, r := range refs {
		if r.Src != "" && !seen[r.Src] {
			seen[r.Src] = true
			srcs = append(srcs, r.Src)
		}
	}
	merged := make([][]branchRef, len(srcs))
	var wg sync.WaitGroup
	for i, src := range srcs {
		wg.Go(func() {
			var err error
			if merged[i], err = a.mergedBranches(ctx, src); err != nil {
				a.warnf("could not list merged branches for %s: %v", src, err)
			}
		})
	}
	wg.Wait()
	var stale []branchRef
	staleSet := map[branchRef]bool{}
	for _, m := range merged {
		for _, r := range m {
			stale = append(stale, r)
			staleSet[r] = true
		}
	}

	tasks, err := a.Tasks.List(ctx)
	if err != nil {
		return err
	}
	var candidates []Task
	var retired []retiree
	for _, t := range tasks {
		wts, err := a.worktrees(ctx, t)
		if err != nil {
			return err
		}
		left := 0
		var mine []retiree
		for _, r := range wts {
			src := filepath.Join(t.Base, r)
			s, err := a.repoStatus(ctx, t, r)
			if err != nil {
				return err
			}
			if s.Orphaned {
				a.warnf("%s/%s is orphaned from %s, skipping", t.Topic, r, src)
				left++
				continue
			}
			s.PR = prs.state(ctx, src, s.Branch)
			clean := s.Dirty == 0 && s.PR.Done()
			if clean {
				kids, err := a.liveChildren(ctx, src, filepath.Join(t.Dir, r), staleSet)
				if err != nil {
					return err
				}
				clean = len(kids) == 0
			}
			if clean {
				mine = append(mine, retiree{task: t, repo: r, branch: s.Branch})
			} else {
				left++
			}
		}
		if len(wts) > 0 && left == 0 {
			candidates = append(candidates, t)
		} else {
			retired = append(retired, mine...)
		}
	}

	if len(candidates) == 0 && len(retired) == 0 && len(stale) == 0 {
		a.logf("nothing to clean up")
		return nil
	}
	if len(candidates) > 0 {
		var rows [][]string
		for _, t := range candidates {
			wts, err := a.worktrees(ctx, t)
			if err != nil {
				return err
			}
			rows = append(rows, []string{"  " + t.Topic, filepath.Base(t.Base), strings.Join(wts, " ")})
		}
		if err := a.section("tasks whose PRs are all merged or closed:", rows); err != nil {
			return err
		}
	}
	if len(retired) > 0 {
		var rows [][]string
		for _, r := range retired {
			rows = append(rows, []string{"  " + r.task.Topic, r.repo, r.branch})
		}
		if err := a.section("worktrees whose PRs are merged or closed:", rows); err != nil {
			return err
		}
	}
	if len(stale) > 0 {
		var rows [][]string
		for _, r := range stale {
			rows = append(rows, []string{"  " + filepath.Base(r.Src), r.Branch})
		}
		if err := a.section("branches with merged PRs:", rows); err != nil {
			return err
		}
	}

	ok, err := a.Prompt.Confirm(ctx, "Remove all of these?")
	if err != nil {
		return err
	}
	if !ok {
		return ErrAborted
	}
	for _, r := range stale {
		repo, err := a.Git.Open(r.Src)
		if err != nil {
			return err
		}
		if err := repo.DeleteBranch(ctx, r.Branch); err != nil {
			return err
		}
		a.logf("removed %s branch %s", filepath.Base(r.Src), r.Branch)
	}
	for _, r := range retired {
		if err := a.retireWorktree(ctx, r); err != nil {
			return err
		}
	}
	if err := a.Trash.Empty(); err != nil {
		return err
	}
	for _, t := range candidates {
		if err := a.removeTask(ctx, t, removeOptions{}); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) section(title string, rows [][]string) error {
	if _, err := a.Out.Write([]byte(title + "\n")); err != nil {
		return err
	}
	return a.table(rows)
}

func worktreeBranches(ctx context.Context, repo Repo) (map[string]bool, error) {
	wts, err := repo.Worktrees(ctx)
	if err != nil {
		return nil, err
	}
	busy := make(map[string]bool, len(wts))
	for _, w := range wts {
		if w.Branch != "" {
			busy[w.Branch] = true
		}
	}
	return busy, nil
}

func (a *App) mergedBranches(ctx context.Context, src string) ([]branchRef, error) {
	repo, err := a.Git.Open(src)
	if err != nil {
		return nil, err
	}
	forge, err := a.Forges.For(ctx, repo)
	if err != nil {
		return nil, err
	}
	merged, err := forge.MergedBranches(ctx, repo)
	if err != nil {
		return nil, err
	}
	busy, err := worktreeBranches(ctx, repo)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var refs []branchRef
	for _, m := range merged {
		if busy[m.Branch] || seen[m.Branch] {
			continue
		}
		sha, ok, err := repo.RevParse(ctx, "refs/heads/"+m.Branch)
		if err != nil {
			return nil, err
		}
		if ok && sha == m.HeadSHA {
			seen[m.Branch] = true
			refs = append(refs, branchRef{Src: src, Branch: m.Branch})
		}
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].Branch < refs[j].Branch })
	return refs, nil
}

func (a *App) liveChildren(ctx context.Context, src, wt string, stale map[branchRef]bool) ([]string, error) {
	srcRepo, err := a.Git.Open(src)
	if err != nil {
		return nil, err
	}
	busy, err := worktreeBranches(ctx, srcRepo)
	if err != nil {
		return nil, err
	}
	def, err := srcRepo.DefaultRef(ctx)
	if err != nil {
		return nil, err
	}
	def = strings.TrimPrefix(def, "origin/")
	wtRepo, err := a.Git.Open(wt)
	if err != nil {
		return nil, err
	}
	containing, err := wtRepo.BranchesContaining(ctx, "HEAD")
	if err != nil {
		return nil, err
	}
	var kids []string
	for _, c := range containing {
		if c != def && !busy[c] && !stale[branchRef{Src: src, Branch: c}] {
			kids = append(kids, c)
		}
	}
	return kids, nil
}

func (a *App) retireWorktree(ctx context.Context, r retiree) error {
	src, err := a.Git.Open(filepath.Join(r.task.Base, r.repo))
	if err != nil {
		return err
	}
	if err := a.discardWorktree(ctx, src, filepath.Join(r.task.Dir, r.repo), false); err != nil {
		return err
	}
	if err := src.DeleteBranch(ctx, r.branch); err != nil {
		return err
	}
	if err := a.Tasks.Link(ctx, r.task, r.repo); err != nil {
		return err
	}
	id, found, err := a.Mux.FindWindow(ctx, r.task.Session, r.repo)
	if err != nil {
		return err
	}
	if found {
		if err := a.Mux.KillWindow(ctx, id); err != nil {
			return err
		}
	}
	if err := a.writeContext(ctx, r.task); err != nil {
		return err
	}
	a.logf("removed %s/%s worktree and branch %s", r.task.Topic, r.repo, r.branch)
	return nil
}
