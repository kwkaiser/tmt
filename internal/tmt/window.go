package tmt

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
)

type WindowOptions struct {
	Session string
	Name    string
	Dir     string
	Command []string
}

func (a *App) Window(ctx context.Context, o WindowOptions) error {
	if o.Dir == "" {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		o.Dir = dir
	}
	session := o.Session
	if session == "" {
		cur, err := a.Mux.CurrentSession(ctx)
		if err != nil {
			return err
		}
		if cur == "" {
			return errors.New("not inside a multiplexer session; pass --session")
		}
		session = cur
	}

	id, found, err := a.Mux.FindWindow(ctx, session, o.Name)
	if err != nil {
		return err
	}
	if !found {
		if id, err = a.Mux.NewWindow(ctx, session, Window{Name: o.Name, Dir: o.Dir}); err != nil {
			return err
		}
	}
	if len(o.Command) == 0 {
		return a.Mux.SelectWindow(ctx, id)
	}

	command := shellCommand(o.Command)
	pane, cd, err := a.commandPane(ctx, id, found, o.Dir)
	if err != nil {
		return err
	}
	if cd {
		command = "cd " + shellWord(o.Dir) + " && " + command
	}
	if err := a.Mux.RunInPane(ctx, pane, command); err != nil {
		return err
	}
	return a.Mux.SelectPane(ctx, pane)
}

func (a *App) commandPane(ctx context.Context, id WindowID, existing bool, dir string) (PaneID, bool, error) {
	panes, err := a.Mux.Panes(ctx, id)
	if err != nil {
		return "", false, err
	}
	if !existing {
		if len(panes) == 0 {
			return "", false, fmt.Errorf("window %s has no panes", id)
		}
		return panes[0].ID, false, nil
	}
	for _, p := range panes {
		if p.Idle {
			return p.ID, true, nil
		}
	}
	pane, err := a.Mux.SplitWindow(ctx, id, dir)
	return pane, false, err
}

var bareWord = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

func shellWord(s string) string {
	if bareWord.MatchString(s) {
		return s
	}
	return shellQuote(s)
}

func shellCommand(args []string) string {
	words := make([]string, len(args))
	for i, s := range args {
		words[i] = shellWord(s)
	}
	return strings.Join(words, " ")
}
