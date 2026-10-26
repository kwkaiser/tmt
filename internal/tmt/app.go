package tmt

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
)

var (
	ErrAborted = errors.New("aborted")
	ErrUnsafe  = errors.New("pass -f to force")
)

const (
	rootWindow   = "claude"
	iconWorktree = "\ue725"
	iconRoot     = "\uf120"
	iconMain     = "\uf07b"
)

type Theme struct {
	Worktree string
	Main     string
	Root     string
	Accent   string
	Dim      string
}

type Config struct {
	Root         string
	BranchPrefix string
	CloneNames   []string
	Self         string
	Theme        Theme
}

type App struct {
	Config Config
	Tasks  TaskStore
	Git    RepoOpener
	Forges ForgeResolver
	Mux    Mux
	Picker Picker
	Prompt Prompter
	Cloner Cloner
	Trash  Trash
	Dirs   DirIndex
	Agent  Agent
	Env    EnvTrust
	Out    io.Writer
	Err    io.Writer
}

func (a *App) logf(format string, args ...any) {
	fmt.Fprintf(a.Out, "tmt: "+format+"\n", args...)
}

func (a *App) warnf(format string, args ...any) {
	fmt.Fprintf(a.Err, "tmt: "+format+"\n", args...)
}

func (a *App) table(rows [][]string) error {
	w := tabwriter.NewWriter(a.Out, 0, 0, 2, ' ', 0)
	for _, r := range rows {
		fmt.Fprintln(w, strings.Join(r, "\t"))
	}
	return w.Flush()
}

func paint(code, s string) string {
	return "\033[" + code + "m" + s + "\033[0m"
}

func (a *App) currentTask(ctx context.Context) (Task, error) {
	if s, err := a.Mux.CurrentSession(ctx); err == nil && s != "" {
		t, err := a.Tasks.BySession(ctx, s)
		if err == nil {
			return t, nil
		}
		if !errors.Is(err, ErrTaskNotFound) {
			return Task{}, err
		}
	}
	wd, err := os.Getwd()
	if err != nil {
		return Task{}, err
	}
	return a.Tasks.Containing(ctx, wd)
}

func (a *App) resolveTask(ctx context.Context, topic string) (Task, error) {
	if topic != "" {
		return a.Tasks.ByTopic(ctx, topic)
	}
	t, err := a.currentTask(ctx)
	if errors.Is(err, ErrTaskNotFound) {
		return Task{}, errors.New("not inside a task; pass a topic")
	}
	return t, err
}

func (a *App) entries(ctx context.Context, t Task, kind EntryKind) ([]string, error) {
	all, err := a.Tasks.Entries(ctx, t)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range all {
		if e.Kind == kind {
			names = append(names, e.Name)
		}
	}
	return names, nil
}

func (a *App) worktrees(ctx context.Context, t Task) ([]string, error) {
	return a.entries(ctx, t, EntryWorktree)
}

func (a *App) resolveBase(ctx context.Context, query string) (string, error) {
	if query == "~" || strings.HasPrefix(query, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		query = home + query[1:]
	}
	if fi, err := os.Stat(query); err == nil && fi.IsDir() {
		return filepath.Abs(query)
	}
	dir, err := a.Dirs.Query(ctx, query)
	if err != nil || dir == "" {
		return "", fmt.Errorf("could not resolve %q to a directory", query)
	}
	return dir, nil
}

func (a *App) baseRepos(base string, limit int) ([]string, error) {
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, err
	}
	var repos []string
	for _, e := range entries {
		p := filepath.Join(base, e.Name())
		if fi, err := os.Stat(p); err != nil || !fi.IsDir() || !a.Git.IsRepo(p) {
			continue
		}
		repos = append(repos, e.Name())
		if limit > 0 && len(repos) == limit {
			break
		}
	}
	return repos, nil
}

func (a *App) checkRepos(base string, repos []string) error {
	var missing []string
	for _, r := range repos {
		if !a.Git.IsRepo(filepath.Join(base, r)) {
			missing = append(missing, r)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("not a git repo under %s: %s", base, strings.Join(missing, " "))
	}
	return nil
}

func (a *App) writeContext(ctx context.Context, t Task) error {
	wts, err := a.worktrees(ctx, t)
	if err != nil {
		return err
	}
	return a.Agent.WriteContext(ctx, t, wts)
}

func (a *App) ensureSession(ctx context.Context, t Task) error {
	has, err := a.Mux.HasSession(ctx, t.Session)
	if err != nil {
		return err
	}
	if !has {
		if err := a.Agent.Trust(ctx, t.Dir); err != nil {
			return err
		}
	}
	wts, err := a.worktrees(ctx, t)
	if err != nil {
		return err
	}
	windows := make([]Window, len(wts))
	for i, r := range wts {
		windows[i] = Window{Name: r, Dir: filepath.Join(t.Dir, r)}
	}
	return a.Mux.EnsureSession(ctx, Session{
		Name:     t.Session,
		Dir:      t.Dir,
		Owner:    t.Dir,
		RootName: rootWindow,
		Command:  a.Agent.Command(),
		Windows:  windows,
	})
}

func (a *App) open(ctx context.Context, t Task) error {
	if err := a.ensureSession(ctx, t); err != nil {
		return err
	}
	return a.Mux.Attach(ctx, t.Session)
}
