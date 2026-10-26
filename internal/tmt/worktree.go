package tmt

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type createdBranch struct {
	repo   Repo
	branch string
}

func (a *App) addRepo(ctx context.Context, t Task, name string, created *[]createdBranch) error {
	src := filepath.Join(t.Base, name)
	target := filepath.Join(t.Dir, name)
	if !a.Git.IsRepo(src) {
		return fmt.Errorf("%s is not a git repo", src)
	}

	entries, err := a.Tasks.Entries(ctx, t)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Name != name {
			continue
		}
		if e.Kind == EntryWorktree {
			a.logf("%s is already a worktree", name)
			return nil
		}
		if err := a.Tasks.Unlink(ctx, t, name); err != nil {
			return err
		}
	}

	repo, err := a.Git.Open(src)
	if err != nil {
		return err
	}
	if err := repo.Fetch(ctx, false); err != nil {
		a.warnf("fetch failed for %s, using local refs", name)
	}
	start, err := repo.DefaultRef(ctx)
	if err != nil {
		return err
	}

	opts := AddWorktree{Path: target, Branch: t.Branch, Start: start, Mode: WorktreeExisting}
	local, err := repo.HasBranch(ctx, t.Branch)
	if err != nil {
		return err
	}
	if !local {
		remote, err := repo.HasRemoteBranch(ctx, t.Branch)
		if err != nil {
			return err
		}
		if remote {
			opts.Mode = WorktreeTrackRemote
			opts.Start = "origin/" + t.Branch
		} else {
			opts.Mode = WorktreeNewBranch
		}
	}
	if err := repo.AddWorktree(ctx, opts); err != nil {
		return err
	}
	if opts.Mode != WorktreeExisting && created != nil {
		*created = append(*created, createdBranch{repo: repo, branch: t.Branch})
	}

	if err := a.copyUntracked(ctx, src, target); err != nil {
		return err
	}
	if err := a.Env.Allow(ctx, target); err != nil {
		return err
	}

	has, err := a.Mux.HasSession(ctx, t.Session)
	if err != nil {
		return err
	}
	if has {
		_, found, err := a.Mux.FindWindow(ctx, t.Session, name)
		if err != nil {
			return err
		}
		if !found {
			if _, err := a.Mux.NewWindow(ctx, t.Session, Window{Name: name, Dir: target}); err != nil {
				return err
			}
		}
	}
	a.logf("%s -> %s (%s from %s)", name, target, t.Branch, start)
	return nil
}

func (a *App) copyUntracked(ctx context.Context, src, dst string) error {
	names := make(map[string]bool, len(a.Config.CloneNames))
	for _, n := range a.Config.CloneNames {
		names[n] = true
	}
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == src {
			return nil
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if !names[d.Name()] {
			return nil
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if _, err := os.Lstat(target); errors.Is(err, fs.ErrNotExist) {
			a.logf("  copying %s/%s", filepath.Base(src), rel)
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := a.Cloner.Clone(ctx, p, target); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		if d.IsDir() {
			return filepath.SkipDir
		}
		return nil
	})
}

func (a *App) discardWorktree(ctx context.Context, src Repo, path string, force bool) error {
	if !force {
		wt, err := a.Git.Open(path)
		if err != nil {
			return err
		}
		st, err := wt.Status(ctx, true)
		if err != nil {
			return err
		}
		if st.Dirty > 0 {
			return fmt.Errorf("%s has uncommitted changes", path)
		}
	}
	if err := a.Trash.Discard(path); err != nil {
		return err
	}
	return src.PruneWorktrees(ctx)
}
