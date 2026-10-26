package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

type call struct {
	method string
	args   []any
}

type fakeApp struct {
	calls []call
	err   error
}

func (f *fakeApp) record(method string, args ...any) error {
	f.calls = append(f.calls, call{method, args})
	return f.err
}

func (f *fakeApp) New(_ context.Context, o tmt.NewOptions) error { return f.record("New", o) }
func (f *fakeApp) Add(_ context.Context, topic string, repos []string) error {
	return f.record("Add", topic, repos)
}
func (f *fakeApp) Open(_ context.Context, topic string) error      { return f.record("Open", topic) }
func (f *fakeApp) Ls(_ context.Context, withPR bool) error         { return f.record("Ls", withPR) }
func (f *fakeApp) Fresh(_ context.Context, query string) error     { return f.record("Fresh", query) }
func (f *fakeApp) Done(_ context.Context, o tmt.DoneOptions) error { return f.record("Done", o) }
func (f *fakeApp) Close(context.Context) error                     { return f.record("Close") }
func (f *fakeApp) GC(context.Context) error                        { return f.record("GC") }
func (f *fakeApp) Pick(context.Context) error                      { return f.record("Pick") }
func (f *fakeApp) Switch(context.Context) error                    { return f.record("Switch") }
func (f *fakeApp) Create(context.Context) error                    { return f.record("Create") }
func (f *fakeApp) Preview(_ context.Context, kind, dir, name string) error {
	return f.record("Preview", kind, dir, name)
}
func (f *fakeApp) Complete(_ context.Context, kind, arg string) error {
	return f.record("Complete", kind, arg)
}

var _ app = (*tmt.App)(nil)

type harness struct {
	app     *fakeApp
	builds  int
	closed  int
	stdout  bytes.Buffer
	stderr  bytes.Buffer
	runCode int
}

func execute(t *testing.T, f *fakeApp, args ...string) *harness {
	t.Helper()
	h := &harness{app: f}
	cmd := newRootCmd(func() (app, func() error, error) {
		h.builds++
		return f, func() error { h.closed++; return nil }, nil
	})
	cmd.SetOut(&h.stdout)
	cmd.SetErr(&h.stderr)
	h.runCode = run(context.Background(), cmd, args, &h.stderr)
	return h
}

func TestDispatch(t *testing.T) {
	cases := []struct {
		args []string
		want call
	}{
		{[]string{"create"}, call{"Create", nil}},
		{[]string{"new", "web", "login"}, call{"New", []any{tmt.NewOptions{Base: "web", Topic: "login", Repos: []string{}}}}},
		{[]string{"new", "-B", "feat/x", "web", "login", "api", "ui"}, call{"New", []any{tmt.NewOptions{Base: "web", Topic: "login", Branch: "feat/x", Repos: []string{"api", "ui"}}}}},
		{[]string{"new", "web", "login", "api", "-B", "b"}, call{"New", []any{tmt.NewOptions{Base: "web", Topic: "login", Branch: "b", Repos: []string{"api"}}}}},
		{[]string{"add", "api"}, call{"Add", []any{"", []string{"api"}}}},
		{[]string{"add", "-t", "login", "api", "ui"}, call{"Add", []any{"login", []string{"api", "ui"}}}},
		{[]string{"open"}, call{"Open", []any{""}}},
		{[]string{"open", "login"}, call{"Open", []any{"login"}}},
		{[]string{"ls"}, call{"Ls", []any{false}}},
		{[]string{"ls", "-p"}, call{"Ls", []any{true}}},
		{[]string{"fresh"}, call{"Fresh", []any{""}}},
		{[]string{"fresh", "~/src"}, call{"Fresh", []any{"~/src"}}},
		{[]string{"done"}, call{"Done", []any{tmt.DoneOptions{}}}},
		{[]string{"done", "-f", "-y", "login"}, call{"Done", []any{tmt.DoneOptions{Topic: "login", Force: true, Yes: true}}}},
		{[]string{"done", "-fy"}, call{"Done", []any{tmt.DoneOptions{Force: true, Yes: true}}}},
		{[]string{"done", "login", "-f"}, call{"Done", []any{tmt.DoneOptions{Topic: "login", Force: true}}}},
		{[]string{"done", "-y", "login", "-f"}, call{"Done", []any{tmt.DoneOptions{Topic: "login", Force: true, Yes: true}}}},
		{[]string{"close"}, call{"Close", nil}},
		{[]string{"gc"}, call{"GC", nil}},
		{[]string{"pick"}, call{"Pick", nil}},
		{[]string{"switch"}, call{"Switch", nil}},
		{[]string{"_preview", "task", "/t/web/web-login"}, call{"Preview", []any{"task", "/t/web/web-login", ""}}},
		{[]string{"_preview", "repo", "/t/web/web-login", "api"}, call{"Preview", []any{"repo", "/t/web/web-login", "api"}}},
		{[]string{"_complete", "tasks"}, call{"Complete", []any{"tasks", ""}}},
		{[]string{"_complete", "base-repos", "web"}, call{"Complete", []any{"base-repos", "web"}}},
		{[]string{"_complete", "task-repos", "-x"}, call{"Complete", []any{"task-repos", "-x"}}},
	}
	for _, c := range cases {
		t.Run(strings.Join(c.args, " "), func(t *testing.T) {
			h := execute(t, &fakeApp{}, c.args...)
			if h.runCode != 0 {
				t.Fatalf("code = %d; stderr %q", h.runCode, h.stderr.String())
			}
			if want := []call{c.want}; !reflect.DeepEqual(h.app.calls, want) {
				t.Fatalf("calls = %#v; want %#v", h.app.calls, want)
			}
			if h.builds != 1 || h.closed != 1 {
				t.Fatalf("builds = %d, closed = %d; want 1, 1", h.builds, h.closed)
			}
		})
	}
}

func TestArgErrors(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"new"}, "tmt: usage: tmt new [-B branch] <base> <topic> [repo...]\n"},
		{[]string{"new", "web"}, "tmt: usage: tmt new [-B branch] <base> <topic> [repo...]\n"},
		{[]string{"add"}, "tmt: usage: tmt add [-t topic] <repo...>\n"},
		{[]string{"add", "-t", "login"}, "tmt: usage: tmt add [-t topic] <repo...>\n"},
		{[]string{"open", "a", "b"}, "tmt: usage: tmt open [topic]\n"},
		{[]string{"ls", "x"}, "tmt: usage: tmt ls [-p]\n"},
		{[]string{"fresh", "a", "b"}, "tmt: usage: tmt fresh [base]\n"},
		{[]string{"done", "a", "b"}, "tmt: usage: tmt done [-f] [-y] [topic]\n"},
		{[]string{"done", "a", "-f", "b"}, "tmt: usage: tmt done [-f] [-y] [topic]\n"},
		{[]string{"close", "x"}, "tmt: usage: tmt close\n"},
		{[]string{"_preview", "task"}, "tmt: usage: tmt _preview <kind> <dir> [name]\n"},
		{[]string{"_complete"}, "tmt: usage: tmt _complete <kind> [arg]\n"},
	}
	for _, c := range cases {
		t.Run(strings.Join(c.args, " "), func(t *testing.T) {
			h := execute(t, &fakeApp{}, c.args...)
			if h.runCode != 1 {
				t.Fatalf("code = %d; want 1", h.runCode)
			}
			if got := h.stderr.String(); got != c.want {
				t.Fatalf("stderr = %q; want %q", got, c.want)
			}
			if h.builds != 0 || len(h.app.calls) != 0 {
				t.Fatalf("builds = %d, calls = %v; want none", h.builds, h.app.calls)
			}
		})
	}
}

func TestUnknownFlag(t *testing.T) {
	h := execute(t, &fakeApp{}, "ls", "-x")
	if h.runCode != 1 || !strings.HasPrefix(h.stderr.String(), "tmt: ") {
		t.Fatalf("code = %d, stderr = %q", h.runCode, h.stderr.String())
	}
}

func TestNoArgs(t *testing.T) {
	for _, args := range [][]string{nil, {"bogus"}} {
		h := execute(t, &fakeApp{}, args...)
		if h.runCode != 1 {
			t.Fatalf("%v: code = %d; want 1", args, h.runCode)
		}
		if h.stderr.String() != usage {
			t.Fatalf("%v: stderr = %q; want usage", args, h.stderr.String())
		}
		if h.stdout.Len() != 0 || h.builds != 0 {
			t.Fatalf("%v: stdout = %q, builds = %d", args, h.stdout.String(), h.builds)
		}
	}
}

func TestHelp(t *testing.T) {
	for _, args := range [][]string{{"-h"}, {"--help"}, {"help"}} {
		h := execute(t, &fakeApp{}, args...)
		if h.runCode != 0 {
			t.Fatalf("%v: code = %d; stderr %q", args, h.runCode, h.stderr.String())
		}
		out := h.stdout.String()
		if out != usage {
			t.Fatalf("%v: stdout = %q; want usage", args, out)
		}
		if h.builds != 0 {
			t.Fatalf("%v: built app for help", args)
		}
	}
	for _, sub := range []string{"create", "new", "add", "open", "ls", "fresh", "done", "close", "gc", "pick", "switch"} {
		if !strings.Contains(usage, "tmt "+sub) {
			t.Errorf("usage missing %s", sub)
		}
	}
	if !strings.Contains(usage, "tasks live under $TMT_ROOT/<base name>/<base name>-<topic>") {
		t.Error("usage missing base note")
	}
}

func TestVersion(t *testing.T) {
	h := execute(t, &fakeApp{}, "--version")
	if h.runCode != 0 || !strings.Contains(h.stdout.String(), version) {
		t.Fatalf("code = %d, stdout = %q", h.runCode, h.stdout.String())
	}
}

func TestExitCodes(t *testing.T) {
	cases := []struct {
		err  error
		code int
		msg  string
	}{
		{fmt.Errorf("web-login: api has uncommitted changes; %w", tmt.ErrUnsafe), 2, "tmt: web-login: api has uncommitted changes; pass -f to force\n"},
		{tmt.ErrAborted, 1, "tmt: aborted\n"},
		{errors.New("boom"), 1, "tmt: boom\n"},
	}
	for _, c := range cases {
		h := execute(t, &fakeApp{err: c.err}, "done", "login")
		if h.runCode != c.code {
			t.Errorf("%v: code = %d; want %d", c.err, h.runCode, c.code)
		}
		if got := h.stderr.String(); got != c.msg {
			t.Errorf("%v: stderr = %q; want %q", c.err, got, c.msg)
		}
		if h.closed != 1 {
			t.Errorf("%v: closed = %d; want 1", c.err, h.closed)
		}
	}
}

func TestBuildError(t *testing.T) {
	var stderr bytes.Buffer
	cmd := newRootCmd(func() (app, func() error, error) { return nil, nil, errors.New("no mux") })
	cmd.SetErr(&stderr)
	if code := run(context.Background(), cmd, []string{"gc"}, &stderr); code != 1 {
		t.Fatalf("code = %d; want 1", code)
	}
	if stderr.String() != "tmt: no mux\n" {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestCleanupError(t *testing.T) {
	var stderr bytes.Buffer
	f := &fakeApp{}
	cmd := newRootCmd(func() (app, func() error, error) {
		return f, func() error { return errors.New("close tty") }, nil
	})
	if code := run(context.Background(), cmd, []string{"gc"}, &stderr); code != 1 {
		t.Fatalf("code = %d; want 1", code)
	}
	if stderr.String() != "tmt: close tty\n" {
		t.Fatalf("stderr = %q", stderr.String())
	}
}
