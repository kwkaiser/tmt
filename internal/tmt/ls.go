package tmt

import (
	"context"
	"path/filepath"
	"strconv"
	"sync"
)

func (a *App) Ls(ctx context.Context, withPR bool) error {
	tasks, err := a.Tasks.List(ctx)
	if err != nil {
		return err
	}
	prs := a.newPRCache()
	if withPR {
		refs, err := a.taskBranches(ctx)
		if err != nil {
			return err
		}
		prs.prefetch(ctx, refs)
	}

	type row struct {
		task  Task
		repo  string
		first bool
		state repoState
		err   error
	}
	var rows []*row
	for _, t := range tasks {
		wts, err := a.worktrees(ctx, t)
		if err != nil {
			return err
		}
		if len(wts) == 0 {
			rows = append(rows, &row{task: t, first: true})
		}
		for i, r := range wts {
			rows = append(rows, &row{task: t, repo: r, first: i == 0})
		}
	}

	var wg sync.WaitGroup
	for _, r := range rows {
		if r.repo == "" {
			continue
		}
		wg.Go(func() {
			r.state, r.err = a.repoStatus(ctx, r.task, r.repo)
			if r.err == nil && withPR && !r.state.Orphaned {
				r.state.PR = prs.state(ctx, filepath.Join(r.task.Base, r.repo), r.state.Branch)
			}
		})
	}
	wg.Wait()

	header := []string{"TASK", "BASE", "SESSION", "REPO", "BRANCH", "DIRTY", "COMMITS", "UNPUSHED"}
	if withPR {
		header = append(header, "PR")
	}
	table := [][]string{header}
	for _, r := range rows {
		if r.err != nil {
			return r.err
		}
		cells := []string{"", "", ""}
		if r.first {
			session := "down"
			if up, err := a.Mux.HasSession(ctx, r.task.Session); err != nil {
				return err
			} else if up {
				session = "up"
			}
			cells = []string{r.task.Topic, filepath.Base(r.task.Base), session}
		}
		pr := "-"
		switch {
		case r.repo == "":
			cells = append(cells, "-", "-", "-", "-", "-")
		case r.state.Orphaned:
			cells = append(cells, r.repo, "(source missing)", "-", "-", "-")
		default:
			s := r.state
			cells = append(cells, r.repo, s.Branch, strconv.Itoa(s.Dirty), strconv.Itoa(s.Own), strconv.Itoa(s.Unpushed))
			pr = string(s.PR)
		}
		if withPR {
			cells = append(cells, pr)
		}
		table = append(table, cells)
	}
	return a.table(table)
}
