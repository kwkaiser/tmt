package tmt

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

func (a *App) previewArgs(args ...string) []string {
	return append([]string{a.Config.Self, "_preview"}, args...)
}

func (a *App) header(pairs ...string) string {
	parts := make([]string, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, paint(a.Config.Theme.Accent, pairs[i])+" "+pairs[i+1])
	}
	return ":: " + strings.Join(parts, "  ")
}

func (a *App) Switch(ctx context.Context) error {
	tasks, err := a.Tasks.List(ctx)
	if err != nil {
		return err
	}
	if len(tasks) == 0 {
		return fmt.Errorf("no tasks under %s; run tmt create", a.Config.Root)
	}
	w, bw := 0, 0
	for _, t := range tasks {
		w = max(w, len(t.Topic))
		bw = max(bw, len(filepath.Base(t.Base)))
	}

	th := a.Config.Theme
	byDir := make(map[string]Task, len(tasks))
	items := make([]PickItem, 0, len(tasks))
	for _, t := range tasks {
		up, err := a.Mux.HasSession(ctx, t.Session)
		if err != nil {
			return err
		}
		state := paint(th.Main, "closed")
		if up {
			state = paint(th.Worktree, "running")
		}
		byDir[t.Dir] = t
		items = append(items, PickItem{
			Key: t.Dir,
			Display: fmt.Sprintf("%s %-*s  %s  %s", paint(th.Root, iconRoot), w, t.Topic,
				paint(th.Dim, fmt.Sprintf("%-*s", bw, filepath.Base(t.Base))), state),
		})
	}

	picked, ok, err := a.Picker.Pick(ctx, Pick{
		Label:   "Tasks",
		Header:  a.header("<enter>", "open task"),
		Preview: a.previewArgs("task", PickKey, ""),
		Items:   items,
	})
	if err != nil || !ok || len(picked.Items) == 0 {
		return err
	}
	return a.open(ctx, byDir[picked.Items[0].Key])
}

func (a *App) Pick(ctx context.Context) error {
	t, err := a.currentTask(ctx)
	if errors.Is(err, ErrTaskNotFound) {
		return a.Switch(ctx)
	}
	if err != nil {
		return err
	}

	entries, err := a.Tasks.Entries(ctx, t)
	if err != nil {
		return err
	}
	w := len(rootWindow)
	for _, e := range entries {
		w = max(w, len(e.Name))
	}

	th := a.Config.Theme
	items := []PickItem{{
		Key:     rootWindow,
		Display: fmt.Sprintf("%s %-*s  %s", paint(th.Root, iconRoot), w, rootWindow, paint(th.Dim, "task root")),
	}}
	for _, e := range entries {
		switch e.Kind {
		case EntryLink:
			items = append(items, PickItem{
				Key:     e.Name,
				Display: fmt.Sprintf("%s %-*s  %s", paint(th.Main, iconMain), w, e.Name, paint(th.Dim, "main checkout")),
			})
		case EntryWorktree:
			repo, err := a.Git.Open(filepath.Join(t.Dir, e.Name))
			if err != nil {
				return err
			}
			st, err := repo.Status(ctx, false)
			if err != nil {
				return err
			}
			items = append(items, PickItem{
				Key:     e.Name,
				Display: fmt.Sprintf("%s %-*s  %s", paint(th.Worktree, iconWorktree), w, e.Name, paint(th.Worktree, st.Branch)),
			})
		}
	}

	picked, ok, err := a.Picker.Pick(ctx, Pick{
		Label:   t.Topic,
		Header:  a.header("<enter>", "jump", "<ctrl-a>", "make worktree", "<ctrl-t>", "tasks"),
		Preview: a.previewArgs("entry", t.Dir, PickKey),
		Items:   items,
		Expect:  []string{"ctrl-a", "ctrl-t"},
	})
	if err != nil || !ok {
		return err
	}
	if picked.Key == "ctrl-t" {
		return a.Switch(ctx)
	}
	if len(picked.Items) == 0 {
		return nil
	}
	name := picked.Items[0].Key

	if picked.Key == "ctrl-a" && name != rootWindow {
		if err := a.addRepo(ctx, t, name, nil); err != nil {
			return err
		}
		if err := a.writeContext(ctx, t); err != nil {
			return err
		}
	}

	id, found, err := a.Mux.FindWindow(ctx, t.Session, name)
	if err != nil {
		return err
	}
	if !found {
		dir := t.Dir
		if name != rootWindow {
			dir = filepath.Join(t.Dir, name)
		}
		if id, err = a.Mux.NewWindow(ctx, t.Session, Window{Name: name, Dir: dir}); err != nil {
			return err
		}
	}
	return a.Mux.SelectWindow(ctx, id)
}

func (a *App) Preview(ctx context.Context, kind, dir, name string) error {
	th := a.Config.Theme
	switch kind {
	case "task":
		t, err := a.Tasks.Containing(ctx, dir)
		if err != nil {
			return err
		}
		fmt.Fprintf(a.Out, "%s  %s\n\n", paint(th.Accent, t.Topic), paint(th.Dim, t.Base))
		wts, err := a.worktrees(ctx, t)
		if err != nil {
			return err
		}
		for _, r := range wts {
			repo, err := a.Git.Open(filepath.Join(t.Dir, r))
			if err != nil {
				return err
			}
			status, err := repo.RenderStatus(ctx)
			if err != nil {
				return err
			}
			fmt.Fprintf(a.Out, "%s %s\n", paint(th.Worktree, iconWorktree), r)
			for _, line := range strings.Split(strings.TrimRight(status, "\n"), "\n") {
				fmt.Fprintf(a.Out, "  %s\n", line)
			}
			fmt.Fprintln(a.Out)
		}
		return nil
	case "entry":
		if name != rootWindow {
			return a.Preview(ctx, "repo", dir, name)
		}
		t, err := a.Tasks.Containing(ctx, dir)
		if err != nil {
			return err
		}
		text, err := a.Agent.ReadContext(ctx, t)
		if err != nil {
			return err
		}
		_, err = fmt.Fprint(a.Out, text)
		return err
	case "repo":
		repo, err := a.Git.Open(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		status, err := repo.RenderStatus(ctx)
		if err != nil {
			return err
		}
		log, err := repo.RenderLog(ctx, 40)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(a.Out, "%s\n%s", status, log)
		return err
	default:
		return fmt.Errorf("unknown preview %q", kind)
	}
}
