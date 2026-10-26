package tmt

import (
	"context"
	"errors"
)

var ErrTaskNotFound = errors.New("task not found")

type Task struct {
	Dir     string
	Topic   string
	Base    string
	Branch  string
	Session string
}

type EntryKind int

const (
	EntryWorktree EntryKind = iota
	EntryLink
)

type Entry struct {
	Name string
	Kind EntryKind
}

type TaskStore interface {
	List(ctx context.Context) ([]Task, error)
	ByTopic(ctx context.Context, topic string) (Task, error)
	BySession(ctx context.Context, session string) (Task, error)
	Containing(ctx context.Context, path string) (Task, error)
	Create(ctx context.Context, t Task) error
	Remove(ctx context.Context, t Task) error

	Entries(ctx context.Context, t Task) ([]Entry, error)
	Link(ctx context.Context, t Task, repo string) error
	Unlink(ctx context.Context, t Task, repo string) error
	LinkMissing(ctx context.Context, t Task) ([]string, error)
}
