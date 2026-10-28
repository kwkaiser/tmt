package tmt

import (
	"context"
	"path/filepath"
	"sort"
	"sync"
)

type repoState struct {
	Branch   string
	Dirty    int
	Own      int
	Unpushed int
	PR       PRState
	Orphaned bool
}

func (p PRState) Done() bool {
	return p == PRMerged || p == PRClosed
}

func (s repoState) safe() bool {
	return !s.Orphaned && s.Dirty == 0 && (s.Unpushed == 0 || s.PR.Done())
}

func (a *App) orphaned(t Task, name string) bool {
	return !a.Git.IsRepo(filepath.Join(t.Base, name)) || !a.Git.IsRepo(filepath.Join(t.Dir, name))
}

func (a *App) repoStatus(ctx context.Context, t Task, name string) (repoState, error) {
	if a.orphaned(t, name) {
		return repoState{Orphaned: true, PR: PRNone}, nil
	}
	wt, err := a.Git.Open(filepath.Join(t.Dir, name))
	if err != nil {
		return repoState{}, err
	}
	src, err := a.Git.Open(filepath.Join(t.Base, name))
	if err != nil {
		return repoState{}, err
	}
	st, err := wt.Status(ctx, true)
	if err != nil {
		return repoState{}, err
	}
	start, err := src.DefaultRef(ctx)
	if err != nil {
		return repoState{}, err
	}
	own, err := wt.CountCommits(ctx, start, "HEAD")
	if err != nil {
		return repoState{}, err
	}
	unpushed := own
	remote, err := wt.HasRemoteBranch(ctx, st.Branch)
	if err != nil {
		return repoState{}, err
	}
	if remote {
		if unpushed, err = wt.CountCommits(ctx, "origin/"+st.Branch, "HEAD"); err != nil {
			return repoState{}, err
		}
	}
	return repoState{Branch: st.Branch, Dirty: st.Dirty, Own: own, Unpushed: unpushed, PR: PRNone}, nil
}

type branchRef struct {
	Src    string
	Branch string
}

func (a *App) taskBranches(ctx context.Context) ([]branchRef, error) {
	tasks, err := a.Tasks.List(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[branchRef]bool{}
	var refs []branchRef
	for _, t := range tasks {
		wts, err := a.worktrees(ctx, t)
		if err != nil {
			return nil, err
		}
		for _, r := range wts {
			if a.orphaned(t, r) {
				continue
			}
			wt, err := a.Git.Open(filepath.Join(t.Dir, r))
			if err != nil {
				return nil, err
			}
			st, err := wt.Status(ctx, false)
			if err != nil {
				return nil, err
			}
			ref := branchRef{Src: filepath.Join(t.Base, r), Branch: st.Branch}
			if !seen[ref] {
				seen[ref] = true
				refs = append(refs, ref)
			}
		}
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Src != refs[j].Src {
			return refs[i].Src < refs[j].Src
		}
		return refs[i].Branch < refs[j].Branch
	})
	return refs, nil
}

type prCache struct {
	app    *App
	mu     sync.Mutex
	states map[string]map[string]PRState
}

func (a *App) newPRCache() *prCache {
	return &prCache{app: a, states: map[string]map[string]PRState{}}
}

func (c *prCache) has(src string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.states[src]
	return ok
}

func (c *prCache) prefetch(ctx context.Context, refs []branchRef) {
	bySrc := map[string][]string{}
	for _, r := range refs {
		if r.Src == "" {
			continue
		}
		if _, ok := bySrc[r.Src]; !ok {
			bySrc[r.Src] = nil
		}
		if r.Branch != "" {
			bySrc[r.Src] = append(bySrc[r.Src], r.Branch)
		}
	}
	var wg sync.WaitGroup
	for src, branches := range bySrc {
		if c.has(src) {
			continue
		}
		wg.Go(func() { c.fetch(ctx, src, branches) })
	}
	wg.Wait()
}

func (c *prCache) fetch(ctx context.Context, src string, branches []string) {
	states, err := c.load(ctx, src, branches)
	if err != nil {
		c.app.warnf("could not fetch PRs for %s: %v", src, err)
	}
	c.mu.Lock()
	c.states[src] = states
	c.mu.Unlock()
}

func (c *prCache) load(ctx context.Context, src string, branches []string) (map[string]PRState, error) {
	repo, err := c.app.Git.Open(src)
	if err != nil {
		return nil, err
	}
	forge, err := c.app.Forges.For(ctx, repo)
	if err != nil {
		return nil, err
	}
	return forge.PRStates(ctx, repo, branches)
}

func (c *prCache) state(ctx context.Context, src, branch string) PRState {
	if !c.has(src) {
		c.fetch(ctx, src, []string{branch})
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if s, ok := c.states[src][branch]; ok {
		return s
	}
	return PRNone
}
