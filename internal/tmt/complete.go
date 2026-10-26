package tmt

import (
	"context"
	"fmt"
)

func (a *App) Complete(ctx context.Context, kind, arg string) error {
	var words []string
	switch kind {
	case "tasks":
		tasks, err := a.Tasks.List(ctx)
		if err != nil {
			return err
		}
		for _, t := range tasks {
			words = append(words, t.Topic)
		}
	case "base-repos":
		base, err := a.resolveBase(ctx, arg)
		if err != nil {
			return nil
		}
		if words, err = a.baseRepos(base, 0); err != nil {
			return err
		}
	case "task-repos":
		t, err := a.resolveTask(ctx, arg)
		if err != nil {
			return err
		}
		if words, err = a.entries(ctx, t, EntryLink); err != nil {
			return err
		}
	}
	for _, w := range words {
		fmt.Fprintln(a.Out, w)
	}
	return nil
}
