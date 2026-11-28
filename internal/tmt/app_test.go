package tmt_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"git.kwkaiser.io/kwkaiser/tmt/internal/taskstore"
	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

type fakeMux struct {
	current  string
	live     map[string]bool
	ensured  []tmt.Session
	attached []string
	killed   []string
	statuses []tmt.MuxStatus
}

func (m *fakeMux) CurrentSession(context.Context) (string, error) { return m.current, nil }
func (m *fakeMux) HasSession(_ context.Context, name string) (bool, error) {
	return m.live[name], nil
}
func (m *fakeMux) Sessions(context.Context) ([]string, error) {
	var names []string
	for n, up := range m.live {
		if up {
			names = append(names, n)
		}
	}
	return names, nil
}
func (m *fakeMux) EnsureSession(_ context.Context, s tmt.Session) error {
	m.ensured = append(m.ensured, s)
	return nil
}
func (m *fakeMux) Attach(_ context.Context, name string) error {
	m.attached = append(m.attached, name)
	return errors.New("open terminal failed: not a terminal")
}
func (m *fakeMux) KillSession(_ context.Context, name string) error {
	m.killed = append(m.killed, name)
	return nil
}
func (m *fakeMux) SetStatus(_ context.Context, s tmt.MuxStatus, _ string) error {
	m.statuses = append(m.statuses, s)
	return nil
}
func (m *fakeMux) FindWindow(context.Context, string, string) (tmt.WindowID, bool, error) {
	return "", false, nil
}
func (m *fakeMux) NewWindow(context.Context, string, tmt.Window) (tmt.WindowID, error) {
	return "", nil
}
func (m *fakeMux) SelectWindow(context.Context, tmt.WindowID) error { return nil }
func (m *fakeMux) KillWindow(context.Context, tmt.WindowID) error   { return nil }

type fakeAgent struct {
	written []tmt.Task
}

func (f *fakeAgent) Command(_, promptFile string) string {
	if promptFile == "" {
		return "agent"
	}
	return "agent <" + promptFile
}
func (f *fakeAgent) Trust(context.Context, string) error { return nil }
func (f *fakeAgent) WriteContext(_ context.Context, t tmt.Task, _ []string) error {
	f.written = append(f.written, t)
	return nil
}
func (f *fakeAgent) ReadContext(context.Context, tmt.Task) (string, error) { return "", nil }
func (f *fakeAgent) For(string) tmt.Agent                                  { return f }

type noEnv struct{}

func (noEnv) Allow(context.Context, string) error   { return nil }
func (noEnv) Warm(string)                           {}
func (noEnv) Wrap(_, command string) string         { return command }
func (noEnv) Write(context.Context, tmt.Task) error { return nil }

type noTrash struct{}

func (noTrash) Discard(path string) error { return os.RemoveAll(path) }
func (noTrash) Empty() error              { return nil }

type eofPrompt struct{ asked []string }

func (p *eofPrompt) Confirm(_ context.Context, msg string) (bool, error) {
	p.asked = append(p.asked, msg)
	return false, nil
}
func (p *eofPrompt) Input(context.Context, string, string) (string, bool, error) {
	return "", false, nil
}

type fakeRepo struct {
	tmt.Repo
	path      string
	branch    string
	dirty     int
	own       int
	merged    []tmt.MergedPR
	heads     map[string]string
	deleted   *[]string
	worktrees []tmt.Worktree
}

func (r *fakeRepo) Path() string { return r.path }
func (r *fakeRepo) Status(context.Context, bool) (tmt.Status, error) {
	return tmt.Status{Branch: r.branch, Dirty: r.dirty}, nil
}
func (r *fakeRepo) Branch(context.Context) (string, error)     { return r.branch, nil }
func (r *fakeRepo) DefaultRef(context.Context) (string, error) { return "origin/main", nil }
func (r *fakeRepo) HasRemoteBranch(context.Context, string) (bool, error) {
	return false, nil
}
func (r *fakeRepo) CountCommits(context.Context, string, string) (int, error) { return r.own, nil }
func (r *fakeRepo) Worktrees(context.Context) ([]tmt.Worktree, error)         { return r.worktrees, nil }
func (r *fakeRepo) RevParse(_ context.Context, ref string) (string, bool, error) {
	sha, ok := r.heads[ref]
	return sha, ok, nil
}
func (r *fakeRepo) PruneWorktrees(context.Context) error { return nil }
func (r *fakeRepo) DeleteBranch(_ context.Context, branch string) error {
	*r.deleted = append(*r.deleted, branch)
	return nil
}

type fakeGit struct {
	repos map[string]*fakeRepo
}

func (g *fakeGit) Open(path string) (tmt.Repo, error) {
	if r, ok := g.repos[path]; ok {
		return r, nil
	}
	return nil, errors.New("no repo at " + path)
}
func (g *fakeGit) Find(context.Context, string) (tmt.Repo, bool, error) { return nil, false, nil }
func (g *fakeGit) IsRepo(path string) bool                              { _, ok := g.repos[path]; return ok }

type fakeForge struct {
	states map[string]tmt.PRState
}

func (f *fakeForge) For(context.Context, tmt.Repo) (tmt.Forge, error) { return f, nil }
func (f *fakeForge) PRStates(_ context.Context, _ tmt.Repo, branches []string) (map[string]tmt.PRState, error) {
	out := map[string]tmt.PRState{}
	for _, b := range branches {
		if s, ok := f.states[b]; ok {
			out[b] = s
		}
	}
	return out, nil
}
func (f *fakeForge) MergedBranches(_ context.Context, r tmt.Repo) ([]tmt.MergedPR, error) {
	return r.(*fakeRepo).merged, nil
}

type env struct {
	app    *tmt.App
	root   string
	base   string
	store  *taskstore.Store
	mux    *fakeMux
	agent  *fakeAgent
	git    *fakeGit
	prompt *eofPrompt
	out    *bytes.Buffer
}

var fixedNow = time.Date(2026, 11, 28, 9, 0, 0, 0, time.UTC)

func newEnv(t *testing.T) *env {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	e := &env{
		root:   filepath.Join(dir, "tasks"),
		base:   filepath.Join(dir, "work"),
		mux:    &fakeMux{live: map[string]bool{}},
		agent:  &fakeAgent{},
		git:    &fakeGit{repos: map[string]*fakeRepo{}},
		prompt: &eofPrompt{},
		out:    &bytes.Buffer{},
	}
	if err := os.MkdirAll(e.base, 0o755); err != nil {
		t.Fatal(err)
	}
	e.store = taskstore.New(e.root)
	e.app = &tmt.App{
		Config: tmt.Config{Root: e.root, BranchPrefix: "u/me/", Self: "/bin/tmt"},
		Tasks:  e.store,
		Git:    e.git,
		Forges: &fakeForge{},
		Mux:    e.mux,
		Prompt: e.prompt,
		Trash:  noTrash{},
		Agents: e.agent,
		Env:    noEnv{},
		Shell:  noEnv{},
		Out:    e.out,
		Err:    &bytes.Buffer{},
		Now:    func() time.Time { return fixedNow },
	}
	return e
}

func (e *env) task(t *testing.T, topic string) tmt.Task {
	t.Helper()
	task, err := e.store.ByTopic(context.Background(), topic)
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func (e *env) status(t *testing.T, task tmt.Task) tmt.TaskStatus {
	t.Helper()
	st, ok, err := e.store.Status(context.Background(), task)
	if err != nil || !ok {
		t.Fatalf("Status = %v, %v", ok, err)
	}
	return st
}

func (e *env) addWorktree(t *testing.T, task tmt.Task, name string, r *fakeRepo) {
	t.Helper()
	wt := filepath.Join(task.Dir, name)
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(task.Base, name)
	r.path = src
	e.git.repos[src] = r
	e.git.repos[wt] = r
}

func TestNewDetachDoesNotAttach(t *testing.T) {
	e := newEnv(t)
	err := e.app.New(context.Background(), tmt.NewOptions{Base: e.base, Topic: "login", Prompt: "fix the login bug", Detach: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(e.mux.attached) != 0 {
		t.Fatalf("attached = %v; want none", e.mux.attached)
	}
	if len(e.mux.ensured) != 1 || e.mux.ensured[0].Name != "work-login" {
		t.Fatalf("ensured = %+v", e.mux.ensured)
	}
	if want := "'/bin/tmt' '_fill' 'work-login'"; e.mux.ensured[0].Command != want {
		t.Fatalf("command = %q; want %q", e.mux.ensured[0].Command, want)
	}
	task := e.task(t, "work-login")
	if task.Prompt != "fix the login bug" {
		t.Fatalf("Prompt = %q", task.Prompt)
	}
	if st := e.status(t, task); st.State != tmt.StateCreating {
		t.Fatalf("status = %+v; want creating", st)
	}
	if !strings.Contains(e.out.String(), "created work-login in "+task.Dir) {
		t.Fatalf("out = %q", e.out.String())
	}
}

func TestNewAttaches(t *testing.T) {
	e := newEnv(t)
	err := e.app.New(context.Background(), tmt.NewOptions{Base: e.base, Topic: "login"})
	if err == nil || !reflect.DeepEqual(e.mux.attached, []string{"work-login"}) {
		t.Fatalf("err = %v, attached = %v", err, e.mux.attached)
	}
}

func TestFillPassesPrompt(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if err := e.app.New(ctx, tmt.NewOptions{Base: e.base, Topic: "login", Prompt: "go", Detach: true}); err != nil {
		t.Fatal(err)
	}
	cmd, err := e.app.Fill(ctx, "work-login", nil)
	if err != nil {
		t.Fatal(err)
	}
	task := e.task(t, "work-login")
	if want := "agent <" + filepath.Join(task.Dir, tmt.PromptFile); cmd != want {
		t.Fatalf("Fill = %q; want %q", cmd, want)
	}
	if n := len(e.agent.written); n == 0 || e.agent.written[n-1].Prompt != "go" {
		t.Fatalf("context written = %+v", e.agent.written)
	}
	if st := e.status(t, task); st.State != tmt.StateRunning {
		t.Fatalf("status = %+v; want running", st)
	}
}

func TestFillWithoutPrompt(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if err := e.app.New(ctx, tmt.NewOptions{Base: e.base, Topic: "login", Detach: true}); err != nil {
		t.Fatal(err)
	}
	cmd, err := e.app.Fill(ctx, "work-login", nil)
	if err != nil || cmd != "agent" {
		t.Fatalf("Fill = %q, %v; want agent", cmd, err)
	}
}

func TestOpenWithPrompt(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if err := e.app.New(ctx, tmt.NewOptions{Base: e.base, Topic: "login", Detach: true}); err != nil {
		t.Fatal(err)
	}
	e.mux.ensured = nil
	e.mux.live["work-login"] = true
	if err := e.app.Open(ctx, tmt.OpenOptions{Topic: "work-login", Prompt: "next", Detach: true}); err == nil {
		t.Fatal("expected error opening a live session with a prompt")
	}
	if task := e.task(t, "work-login"); task.Prompt != "" {
		t.Fatalf("Prompt = %q; want unchanged", task.Prompt)
	}

	e.mux.live["work-login"] = false
	if err := e.app.Open(ctx, tmt.OpenOptions{Topic: "work-login", Prompt: "next", Detach: true}); err != nil {
		t.Fatal(err)
	}
	task := e.task(t, "work-login")
	if task.Prompt != "next" {
		t.Fatalf("Prompt = %q; want next", task.Prompt)
	}
	if len(e.mux.ensured) != 1 || e.mux.ensured[0].Command != "agent <"+filepath.Join(task.Dir, tmt.PromptFile) {
		t.Fatalf("ensured = %+v", e.mux.ensured)
	}
	if len(e.mux.attached) != 0 {
		t.Fatalf("attached = %v", e.mux.attached)
	}
	if len(e.agent.written) != 1 || e.agent.written[0].Prompt != "next" {
		t.Fatalf("context written = %+v", e.agent.written)
	}
}

func TestOpenWithoutPromptIgnoresStoredPrompt(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if err := e.app.New(ctx, tmt.NewOptions{Base: e.base, Topic: "login", Prompt: "first", Detach: true}); err != nil {
		t.Fatal(err)
	}
	e.mux.ensured = nil
	if err := e.app.Open(ctx, tmt.OpenOptions{Topic: "work-login", Detach: true}); err != nil {
		t.Fatal(err)
	}
	if len(e.mux.ensured) != 1 || e.mux.ensured[0].Command != "agent" {
		t.Fatalf("ensured = %+v", e.mux.ensured)
	}
}

func TestLsJSON(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if err := e.app.New(ctx, tmt.NewOptions{Base: e.base, Topic: "login", Detach: true}); err != nil {
		t.Fatal(err)
	}
	if err := e.app.New(ctx, tmt.NewOptions{Base: e.base, Topic: "empty", Detach: true}); err != nil {
		t.Fatal(err)
	}
	task := e.task(t, "work-login")
	var deleted []string
	e.addWorktree(t, task, "api", &fakeRepo{branch: "u/me/login-1", dirty: 2, own: 3, deleted: &deleted})
	if err := os.MkdirAll(filepath.Join(task.Dir, "ghost"), 0o755); err != nil {
		t.Fatal(err)
	}
	e.app.Forges = &fakeForge{states: map[string]tmt.PRState{"u/me/login-1": tmt.PROpen}}
	e.mux.live["work-login"] = true
	if err := e.app.SetAgentStatus(ctx, tmt.StatusOptions{Topic: "work-login", State: tmt.StateBlocked, Message: "need input"}); err != nil {
		t.Fatal(err)
	}
	e.out.Reset()

	if err := e.app.Ls(ctx, tmt.LsOptions{PR: true, JSON: true}); err != nil {
		t.Fatal(err)
	}
	var got []map[string]any
	if err := json.Unmarshal(e.out.Bytes(), &got); err != nil {
		t.Fatalf("%v\n%s", err, e.out.String())
	}
	if len(got) != 2 {
		t.Fatalf("got %d tasks; want 2\n%s", len(got), e.out.String())
	}
	byTopic := map[string]map[string]any{}
	for _, g := range got {
		byTopic[g["topic"].(string)] = g
	}
	login := byTopic["work-login"]
	if login["session_up"] != true || login["base"] != e.base || login["dir"] != task.Dir || login["branch"] != "u/me/login-1" {
		t.Fatalf("login = %v", login)
	}
	if want := map[string]any{"state": "blocked", "message": "need input", "updated": "2026-11-28T09:00:00Z"}; !reflect.DeepEqual(login["status"], want) {
		t.Fatalf("status = %v; want %v", login["status"], want)
	}
	wantRepos := []any{
		map[string]any{"name": "api", "branch": "u/me/login-1", "dirty": 2.0, "commits": 3.0, "unpushed": 3.0, "orphaned": false, "pr": "OPEN"},
		map[string]any{"name": "ghost", "dirty": 0.0, "commits": 0.0, "unpushed": 0.0, "orphaned": true},
	}
	if !reflect.DeepEqual(login["repos"], wantRepos) {
		t.Fatalf("repos = %v; want %v", login["repos"], wantRepos)
	}
	empty := byTopic["work-empty"]
	if empty["session_up"] != false || !reflect.DeepEqual(empty["repos"], []any{}) {
		t.Fatalf("empty = %v", empty)
	}
	if st := empty["status"].(map[string]any); st["state"] != "creating" {
		t.Fatalf("empty status = %v", st)
	}
}

func TestLsJSONNoTasks(t *testing.T) {
	e := newEnv(t)
	if err := e.app.Ls(context.Background(), tmt.LsOptions{JSON: true}); err != nil {
		t.Fatal(err)
	}
	if got := e.out.String(); got != "[]\n" {
		t.Fatalf("out = %q; want []", got)
	}
}

func TestLsTable(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if err := e.app.New(ctx, tmt.NewOptions{Base: e.base, Topic: "login", Detach: true}); err != nil {
		t.Fatal(err)
	}
	task := e.task(t, "work-login")
	var deleted []string
	e.addWorktree(t, task, "api", &fakeRepo{branch: "b", own: 1, deleted: &deleted})
	e.addWorktree(t, task, "web", &fakeRepo{branch: "b", deleted: &deleted})
	e.out.Reset()
	if err := e.app.Ls(ctx, tmt.LsOptions{}); err != nil {
		t.Fatal(err)
	}
	want := "TASK        BASE  SESSION  REPO  BRANCH  DIRTY  COMMITS  UNPUSHED\n" +
		"work-login  work  down     api   b       0      1        1\n" +
		"                           web   b       0      0        0\n"
	if got := e.out.String(); got != want {
		t.Fatalf("out =\n%s\nwant\n%s", got, want)
	}
}

func gcEnv(t *testing.T) (*env, *[]string) {
	t.Helper()
	e := newEnv(t)
	if err := e.app.New(context.Background(), tmt.NewOptions{Base: e.base, Topic: "login", Detach: true}); err != nil {
		t.Fatal(err)
	}
	task := e.task(t, "work-login")
	var deleted []string
	e.addWorktree(t, task, "api", &fakeRepo{
		branch:    "u/me/login-1",
		own:       1,
		merged:    []tmt.MergedPR{{Branch: "old", HeadSHA: "abc"}},
		heads:     map[string]string{"refs/heads/old": "abc"},
		worktrees: []tmt.Worktree{{Branch: "u/me/login-1"}},
		deleted:   &deleted,
	})
	e.app.Forges = &fakeForge{states: map[string]tmt.PRState{"u/me/login-1": tmt.PROpen}}
	return e, &deleted
}

func TestGCWithoutYesAborts(t *testing.T) {
	e, deleted := gcEnv(t)
	if err := e.app.GC(context.Background(), false); !errors.Is(err, tmt.ErrAborted) {
		t.Fatalf("GC = %v; want ErrAborted", err)
	}
	if len(e.prompt.asked) != 1 || len(*deleted) != 0 {
		t.Fatalf("asked = %v, deleted = %v", e.prompt.asked, *deleted)
	}
}

func TestGCYesSkipsConfirm(t *testing.T) {
	e, deleted := gcEnv(t)
	if err := e.app.GC(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if len(e.prompt.asked) != 0 {
		t.Fatalf("asked = %v; want none", e.prompt.asked)
	}
	if !reflect.DeepEqual(*deleted, []string{"old"}) {
		t.Fatalf("deleted = %v; want [old]", *deleted)
	}
}

func TestCloseYesSkipsConfirm(t *testing.T) {
	e := newEnv(t)
	e.mux.current = "scratch"
	if err := e.app.Close(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if len(e.prompt.asked) != 1 || len(e.mux.killed) != 0 {
		t.Fatalf("asked = %v, killed = %v", e.prompt.asked, e.mux.killed)
	}
	e.prompt.asked = nil
	if err := e.app.Close(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if len(e.prompt.asked) != 0 || !reflect.DeepEqual(e.mux.killed, []string{"scratch"}) {
		t.Fatalf("asked = %v, killed = %v", e.prompt.asked, e.mux.killed)
	}
}

func TestCloseYesRefusesUnsafeTask(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if err := e.app.New(ctx, tmt.NewOptions{Base: e.base, Topic: "login", Detach: true}); err != nil {
		t.Fatal(err)
	}
	task := e.task(t, "work-login")
	var deleted []string
	e.addWorktree(t, task, "api", &fakeRepo{branch: "b", dirty: 1, deleted: &deleted})
	e.mux.current = "work-login"
	if err := e.app.Close(ctx, true); !errors.Is(err, tmt.ErrUnsafe) {
		t.Fatalf("Close = %v; want ErrUnsafe", err)
	}
	if len(e.prompt.asked) != 0 || len(e.mux.killed) != 0 {
		t.Fatalf("asked = %v, killed = %v", e.prompt.asked, e.mux.killed)
	}
}

func TestSetAgentStatus(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if err := e.app.New(ctx, tmt.NewOptions{Base: e.base, Topic: "login", Detach: true}); err != nil {
		t.Fatal(err)
	}
	if err := e.app.SetAgentStatus(ctx, tmt.StatusOptions{Topic: "work-login", State: "bogus"}); err == nil {
		t.Fatal("expected error for unknown state")
	}
	if err := e.app.SetAgentStatus(ctx, tmt.StatusOptions{Topic: "work-login", State: tmt.StateFinished, Message: "done"}); err != nil {
		t.Fatal(err)
	}
	want := tmt.TaskStatus{State: tmt.StateFinished, Message: "done", Updated: fixedNow}
	if st := e.status(t, e.task(t, "work-login")); !reflect.DeepEqual(st, want) {
		t.Fatalf("status = %+v; want %+v", st, want)
	}
	if len(e.mux.statuses) != 0 {
		t.Fatalf("mux statuses = %v; want none outside the task session", e.mux.statuses)
	}

	e.mux.current = "work-login"
	if err := e.app.SetAgentStatus(ctx, tmt.StatusOptions{State: tmt.StateBlocked}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(e.mux.statuses, []tmt.MuxStatus{tmt.MuxBlocked}) {
		t.Fatalf("mux statuses = %v", e.mux.statuses)
	}
}

type failingMux struct{ fakeMux }

func (m *failingMux) EnsureSession(context.Context, tmt.Session) error {
	return errors.New("no server")
}

func TestNewRollsBackOnSessionError(t *testing.T) {
	e := newEnv(t)
	e.git.repos[filepath.Join(e.base, "api")] = &fakeRepo{path: filepath.Join(e.base, "api")}
	e.app.Mux = &failingMux{}
	err := e.app.New(context.Background(), tmt.NewOptions{Base: e.base, Topic: "login", Repos: []string{"api"}, Detach: true})
	if err == nil {
		t.Fatal("expected error")
	}
	if _, err := os.Stat(filepath.Join(e.root, "work", "work-login")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("task dir left behind: %v", err)
	}
}
