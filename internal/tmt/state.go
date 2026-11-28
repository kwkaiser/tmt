package tmt

import (
	"context"
	"fmt"
	"time"
)

type StatusOptions struct {
	Topic   string
	State   AgentState
	Message string
}

var muxStates = map[AgentState]MuxStatus{
	StateRunning:  MuxReady,
	StateBlocked:  MuxBlocked,
	StateFinished: MuxReady,
}

func (a *App) recordStatus(ctx context.Context, t Task, s AgentState, msg string) {
	if err := a.Tasks.SetStatus(ctx, t, TaskStatus{State: s, Message: msg, Updated: a.now()}); err != nil {
		a.warnf("%v", err)
	}
}

func (a *App) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

func (a *App) SetAgentStatus(ctx context.Context, o StatusOptions) error {
	ms, ok := muxStates[o.State]
	if !ok {
		return fmt.Errorf("unknown state %q; want running, blocked or finished", o.State)
	}
	t, err := a.resolveTask(ctx, o.Topic)
	if err != nil {
		return err
	}
	if err := a.Tasks.SetStatus(ctx, t, TaskStatus{State: o.State, Message: o.Message, Updated: a.now()}); err != nil {
		return err
	}
	if current, err := a.Mux.CurrentSession(ctx); err == nil && current == t.Session {
		a.setStatus(ctx, ms, o.Message)
	}
	return nil
}
