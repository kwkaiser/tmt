package tmt

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
)

func (a *App) Fresh(ctx context.Context, query string) error {
	var base string
	if query != "" {
		b, err := a.resolveBase(ctx, query)
		if err != nil {
			return err
		}
		base = b
	} else {
		t, err := a.currentTask(ctx)
		if errors.Is(err, ErrTaskNotFound) {
			return errors.New("usage: tmt fresh [base]")
		}
		if err != nil {
			return err
		}
		base = t.Base
	}

	repos, err := a.baseRepos(base, 0)
	if err != nil {
		return err
	}
	a.logf("fetching %d repos in %s", len(repos), base)
	rows := make([][]string, len(repos))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i, r := range repos {
		wg.Go(func() {
			out := &lineWriter{mu: &mu, w: a.Err, prefix: r + ": "}
			rows[i] = a.freshRepo(ctx, filepath.Join(base, r), out)
			out.flush()
			out.line(rows[i][2])
		})
	}
	wg.Wait()
	if err := a.table(rows); err != nil {
		return err
	}

	tasks, err := a.Tasks.List(ctx)
	if err != nil {
		return err
	}
	for _, t := range tasks {
		if t.Base != base {
			continue
		}
		linked, err := a.Tasks.LinkMissing(ctx, t)
		if err != nil {
			return err
		}
		for _, r := range linked {
			a.logf("linked %s into %s", r, t.Topic)
		}
		if err := a.writeContext(ctx, t); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) freshRepo(ctx context.Context, src string, progress io.Writer) []string {
	name := filepath.Base(src)
	row := func(branch, msg string) []string { return []string{name, branch, msg} }

	repo, err := a.Git.Open(src)
	if err != nil {
		return row("-", err.Error())
	}
	if err := repo.Fetch(ctx, true, progress); err != nil {
		return row("-", "fetch failed")
	}
	def, err := repo.DefaultRef(ctx)
	if err != nil || def == "HEAD" {
		return row("-", "fetched, no origin default branch")
	}
	def = strings.TrimPrefix(def, "origin/")
	if has, err := repo.HasBranch(ctx, def); err != nil || !has {
		return row(def, "fetched, no local branch")
	}

	before, _, err := repo.RevParse(ctx, def)
	if err != nil {
		return row(def, err.Error())
	}
	upstream := "origin/" + def
	target := repo
	wts, err := repo.Worktrees(ctx)
	if err != nil {
		return row(def, err.Error())
	}
	for _, w := range wts {
		if w.Branch != def {
			continue
		}
		if target, err = a.Git.Open(w.Path); err != nil {
			return row(def, err.Error())
		}
		st, err := target.Status(ctx, false)
		if err != nil {
			return row(def, err.Error())
		}
		if st.Dirty > 0 {
			return row(def, "skipped: checked out with uncommitted changes")
		}
		break
	}
	if err := target.FastForward(ctx, def, upstream); err != nil {
		return row(def, "skipped: diverged from "+upstream)
	}
	after, _, err := repo.RevParse(ctx, def)
	if err != nil {
		return row(def, err.Error())
	}

	n, err := repo.CountCommits(ctx, before, after)
	switch {
	case err != nil:
		return row(def, err.Error())
	case n == 0:
		return row(def, "up to date")
	case n == 1:
		return row(def, "+1 commit")
	default:
		return row(def, fmt.Sprintf("+%d commits", n))
	}
}

type lineWriter struct {
	mu     *sync.Mutex
	w      io.Writer
	prefix string
	buf    []byte
}

func (l *lineWriter) Write(p []byte) (int, error) {
	l.buf = append(l.buf, p...)
	for {
		i := bytes.IndexAny(l.buf, "\r\n")
		if i < 0 {
			return len(p), nil
		}
		l.line(string(l.buf[:i]))
		l.buf = l.buf[i+1:]
	}
}

func (l *lineWriter) flush() {
	l.line(string(l.buf))
	l.buf = nil
}

func (l *lineWriter) line(s string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintln(l.w, l.prefix+s)
}
