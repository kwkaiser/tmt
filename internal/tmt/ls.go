package tmt

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"time"
)

type LsOptions struct {
	PR   bool
	JSON bool
}

type lsRepo struct {
	Name  string
	State repoState
	err   error
}

type lsTask struct {
	Task      Task
	SessionUp bool
	Status    TaskStatus
	HasStatus bool
	Repos     []*lsRepo
}

func (a *App) Ls(ctx context.Context, o LsOptions) error {
	tasks, err := a.listTasks(ctx, o.PR)
	if err != nil {
		return err
	}
	if o.JSON {
		return a.lsJSON(tasks, o.PR)
	}
	return a.lsTable(tasks, o.PR)
}

func (a *App) listTasks(ctx context.Context, withPR bool) ([]*lsTask, error) {
	tasks, err := a.Tasks.List(ctx)
	if err != nil {
		return nil, err
	}
	prs := a.newPRCache()
	if withPR {
		refs, err := a.taskBranches(ctx)
		if err != nil {
			return nil, err
		}
		prs.prefetch(ctx, refs)
	}

	live, err := a.liveSessions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*lsTask, len(tasks))
	for i, t := range tasks {
		wts, err := a.worktrees(ctx, t)
		if err != nil {
			return nil, err
		}
		st, ok, err := a.Tasks.Status(ctx, t)
		if err != nil {
			return nil, err
		}
		lt := &lsTask{Task: t, SessionUp: live[t.Session], Status: st, HasStatus: ok}
		for _, r := range wts {
			lt.Repos = append(lt.Repos, &lsRepo{Name: r})
		}
		out[i] = lt
	}

	var wg sync.WaitGroup
	for _, lt := range out {
		for _, r := range lt.Repos {
			wg.Go(func() {
				r.State, r.err = a.repoStatus(ctx, lt.Task, r.Name)
				if r.err == nil && withPR && !r.State.Orphaned {
					r.State.PR = prs.state(ctx, filepath.Join(lt.Task.Base, r.Name), r.State.Branch)
				}
			})
		}
	}
	wg.Wait()
	for _, lt := range out {
		for _, r := range lt.Repos {
			if r.err != nil {
				return nil, r.err
			}
		}
	}
	return out, nil
}

func sessionState(up bool) string {
	if up {
		return "up"
	}
	return "down"
}

func (a *App) lsTable(tasks []*lsTask, withPR bool) error {
	header := []string{"TASK", "BASE", "SESSION", "REPO", "BRANCH", "DIRTY", "COMMITS", "UNPUSHED"}
	if withPR {
		header = append(header, "PR")
	}
	table := [][]string{header}
	for _, lt := range tasks {
		lead := []string{lt.Task.Topic, filepath.Base(lt.Task.Base), sessionState(lt.SessionUp)}
		if len(lt.Repos) == 0 {
			cells := slices.Concat(lead, []string{"-", "-", "-", "-", "-"})
			if withPR {
				cells = append(cells, "-")
			}
			table = append(table, cells)
		}
		for i, r := range lt.Repos {
			cells := []string{"", "", ""}
			if i == 0 {
				cells = lead
			}
			pr := "-"
			if r.State.Orphaned {
				cells = append(cells, r.Name, "(orphaned)", "-", "-", "-")
			} else {
				s := r.State
				cells = append(cells, r.Name, s.Branch, strconv.Itoa(s.Dirty), strconv.Itoa(s.Own), strconv.Itoa(s.Unpushed))
				pr = string(s.PR)
			}
			if withPR {
				cells = append(cells, pr)
			}
			table = append(table, cells)
		}
	}
	return a.table(table)
}

type jsonTask struct {
	Topic     string      `json:"topic"`
	Base      string      `json:"base"`
	Dir       string      `json:"dir"`
	Branch    string      `json:"branch"`
	Session   string      `json:"session"`
	SessionUp bool        `json:"session_up"`
	Status    *jsonStatus `json:"status"`
	Repos     []jsonRepo  `json:"repos"`
}

type jsonStatus struct {
	State   AgentState `json:"state"`
	Message string     `json:"message,omitempty"`
	Updated *time.Time `json:"updated,omitempty"`
}

type jsonRepo struct {
	Name     string  `json:"name"`
	Branch   string  `json:"branch,omitempty"`
	Dirty    int     `json:"dirty"`
	Commits  int     `json:"commits"`
	Unpushed int     `json:"unpushed"`
	Orphaned bool    `json:"orphaned"`
	PR       PRState `json:"pr,omitempty"`
}

func (a *App) lsJSON(tasks []*lsTask, withPR bool) error {
	out := make([]jsonTask, len(tasks))
	for i, lt := range tasks {
		jt := jsonTask{
			Topic:     lt.Task.Topic,
			Base:      lt.Task.Base,
			Dir:       lt.Task.Dir,
			Branch:    lt.Task.Branch,
			Session:   lt.Task.Session,
			SessionUp: lt.SessionUp,
			Repos:     make([]jsonRepo, len(lt.Repos)),
		}
		if lt.HasStatus {
			js := &jsonStatus{State: lt.Status.State, Message: lt.Status.Message}
			if !lt.Status.Updated.IsZero() {
				ts := lt.Status.Updated
				js.Updated = &ts
			}
			jt.Status = js
		}
		for j, r := range lt.Repos {
			jr := jsonRepo{Name: r.Name, Orphaned: r.State.Orphaned}
			if !r.State.Orphaned {
				jr.Branch = r.State.Branch
				jr.Dirty = r.State.Dirty
				jr.Commits = r.State.Own
				jr.Unpushed = r.State.Unpushed
				if withPR {
					jr.PR = r.State.PR
				}
			}
			jt.Repos[j] = jr
		}
		out[i] = jt
	}
	enc := json.NewEncoder(a.Out)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
