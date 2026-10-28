package tmt

import (
	"context"
	"errors"
)

var ErrForeignSession = errors.New("session exists but does not belong to this task")

type WindowID string

type Window struct {
	Name string
	Dir  string
}

type Session struct {
	Name     string
	Dir      string
	Owner    string
	RootName string
	Command  string
	Windows  []Window
}

type Mux interface {
	CurrentSession(ctx context.Context) (string, error)
	HasSession(ctx context.Context, name string) (bool, error)
	Sessions(ctx context.Context) ([]string, error)
	EnsureSession(ctx context.Context, s Session) error
	Attach(ctx context.Context, name string) error
	KillSession(ctx context.Context, name string) error

	FindWindow(ctx context.Context, session, name string) (WindowID, bool, error)
	NewWindow(ctx context.Context, session string, w Window) (WindowID, error)
	SelectWindow(ctx context.Context, id WindowID) error
	KillWindow(ctx context.Context, id WindowID) error
}
