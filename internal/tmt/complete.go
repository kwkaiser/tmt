package tmt

import (
	"context"
	"fmt"
)

func (a *App) Complete(ctx context.Context, kind, arg string) error {
	words, err := a.completions(ctx, kind, arg)
	if err != nil {
		return err
	}
	for _, w := range words {
		fmt.Fprintln(a.Out, w)
	}
	return nil
}

func (a *App) completions(ctx context.Context, kind, arg string) ([]string, error) {
	switch kind {
	case "tasks":
		tasks, err := a.Tasks.List(ctx)
		if err != nil {
			return nil, err
		}
		topics := make([]string, len(tasks))
		for i, t := range tasks {
			topics[i] = t.Topic
		}
		return topics, nil
	case "base-repos":
		if base, err := a.resolveBase(ctx, arg); err == nil {
			return a.baseRepos(base, 0)
		}
		return nil, nil
	case "task-repos":
		t, err := a.resolveTask(ctx, arg)
		if err != nil {
			return nil, err
		}
		return a.entries(ctx, t, EntryLink)
	}
	return nil, nil
}
