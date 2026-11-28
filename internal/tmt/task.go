package tmt

import (
	"context"
	"errors"
	"time"
)

var ErrTaskNotFound = errors.New("task not found")

const PromptFile = ".tmt-prompt"

type Task struct {
	Dir     string
	Topic   string
	Base    string
	Branch  string
	Session string
	Prompt  string
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

type AgentState string

const (
	StateCreating AgentState = "creating"
	StateRunning  AgentState = "running"
	StateBlocked  AgentState = "blocked"
	StateFinished AgentState = "finished"
)

type TaskStatus struct {
	State   AgentState
	Message string
	Updated time.Time
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

	SetPrompt(ctx context.Context, t Task, prompt string) error
	SetStatus(ctx context.Context, t Task, s TaskStatus) error
	Status(ctx context.Context, t Task) (TaskStatus, bool, error)
}
