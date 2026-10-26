package tmt

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func (a *App) Create(ctx context.Context) error {
	bases, err := a.candidateBases(ctx)
	if err != nil {
		return err
	}
	th := a.Config.Theme
	items := make([]PickItem, len(bases))
	for i, d := range bases {
		items[i] = PickItem{Key: d, Display: fmt.Sprintf("%s %s  %s", paint(th.Root, iconRoot), filepath.Base(d), paint(th.Dim, d))}
	}
	picked, ok, err := a.Picker.Pick(ctx, Pick{
		Label:   "Base",
		Header:  a.header("<enter>", "pick base"),
		Preview: []string{a.Config.Self, "_complete", "base-repos", PickKey},
		Items:   items,
	})
	if err != nil || !ok || len(picked.Items) == 0 {
		return err
	}
	base := picked.Items[0].Key

	topic, ok, err := a.Prompt.Input(ctx, "topic: ", topicPattern.String())
	if err != nil || !ok || topic == "" {
		return err
	}

	repos, err := a.baseRepos(base, 0)
	if err != nil {
		return err
	}
	items = make([]PickItem, len(repos))
	for i, r := range repos {
		items[i] = PickItem{Key: r, Display: paint(th.Worktree, iconWorktree) + " " + r}
	}
	picked, ok, err = a.Picker.Pick(ctx, Pick{
		Label:   topic,
		Header:  a.header("<tab>", "select", "<enter>", "create worktrees"),
		Preview: a.previewArgs("repo", base, PickKey),
		Items:   items,
		Multi:   true,
	})
	if err != nil || !ok || len(picked.Items) == 0 {
		return err
	}
	selected := make([]string, len(picked.Items))
	for i, it := range picked.Items {
		selected[i] = it.Key
	}
	return a.New(ctx, NewOptions{Base: base, Topic: topic, Repos: selected})
}

func (a *App) candidateBases(ctx context.Context) ([]string, error) {
	var dirs []string
	t, err := a.currentTask(ctx)
	switch {
	case err == nil:
		dirs = append(dirs, t.Base)
	case !errors.Is(err, ErrTaskNotFound):
		return nil, err
	}
	if wd, err := os.Getwd(); err == nil {
		if repo, ok, err := a.Git.Find(wd); err == nil && ok {
			dirs = append(dirs, filepath.Dir(repo.Path()))
		}
	}
	tasks, err := a.Tasks.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, t := range tasks {
		dirs = append(dirs, t.Base)
	}
	if indexed, err := a.Dirs.List(ctx); err == nil {
		dirs = append(dirs, indexed...)
	}

	seen := map[string]bool{}
	var bases []string
	for _, d := range dirs {
		if seen[d] {
			continue
		}
		seen[d] = true
		if repos, err := a.baseRepos(d, 1); err == nil && len(repos) > 0 {
			bases = append(bases, d)
		}
	}
	return bases, nil
}
