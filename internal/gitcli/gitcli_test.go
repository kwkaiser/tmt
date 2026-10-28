package gitcli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

func hermetic(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CEILING_DIRECTORIES", root)
	t.Setenv("GIT_AUTHOR_NAME", "tmt")
	t.Setenv("GIT_AUTHOR_EMAIL", "tmt@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "tmt")
	t.Setenv("GIT_COMMITTER_EMAIL", "tmt@example.com")
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "init.defaultBranch")
	t.Setenv("GIT_CONFIG_VALUE_0", "main")
	return root
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func commit(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", name)
	git(t, dir, "commit", "--quiet", "-m", name)
}

type fixture struct {
	root   string
	origin string
	seed   string
	clone  string
}

func setup(t *testing.T) fixture {
	t.Helper()
	root := hermetic(t)
	f := fixture{
		root:   root,
		origin: filepath.Join(root, "origin.git"),
		seed:   filepath.Join(root, "seed"),
		clone:  filepath.Join(root, "clone"),
	}
	git(t, root, "init", "--quiet", "--bare", f.origin)
	git(t, root, "clone", "--quiet", f.origin, f.seed)
	commit(t, f.seed, "a")
	git(t, f.seed, "push", "--quiet", "origin", "main")
	git(t, f.seed, "checkout", "--quiet", "-b", "feature")
	commit(t, f.seed, "b")
	git(t, f.seed, "push", "--quiet", "origin", "feature")
	git(t, f.seed, "checkout", "--quiet", "main")
	git(t, root, "clone", "--quiet", f.origin, f.clone)
	return f
}

func open(t *testing.T, path string) tmt.Repo {
	t.Helper()
	r, err := New().Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func pushMain(t *testing.T, f fixture, name string) {
	t.Helper()
	commit(t, f.seed, name)
	git(t, f.seed, "push", "--quiet", "origin", "main")
}

func TestDefaultRef(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	ref, err := open(t, f.clone).DefaultRef(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if ref != "origin/main" {
		t.Fatalf("DefaultRef = %q, want origin/main", ref)
	}

	git(t, f.clone, "remote", "set-head", "origin", "--delete")
	if ref, err = open(t, f.clone).DefaultRef(ctx); err != nil || ref != "origin/main" {
		t.Fatalf("DefaultRef without origin/HEAD = %q, %v", ref, err)
	}

	local := filepath.Join(f.root, "local")
	git(t, f.root, "init", "--quiet", local)
	commit(t, local, "x")
	if ref, err = open(t, local).DefaultRef(ctx); err != nil || ref != "HEAD" {
		t.Fatalf("DefaultRef with no origin = %q, %v", ref, err)
	}
}

func TestRemoteURLAndFetch(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	r := open(t, f.clone)
	url, err := r.RemoteURL(ctx, "origin")
	if err != nil || url != f.origin {
		t.Fatalf("RemoteURL = %q, %v", url, err)
	}
	if _, err := r.RemoteURL(ctx, "nope"); err == nil {
		t.Fatal("RemoteURL for missing remote succeeded")
	}

	git(t, f.seed, "push", "--quiet", "origin", "--delete", "feature")
	if err := r.Fetch(ctx, false); err != nil {
		t.Fatal(err)
	}
	if has, _ := r.HasRemoteBranch(ctx, "feature"); !has {
		t.Fatal("feature pruned without --prune")
	}
	if err := r.Fetch(ctx, true); err != nil {
		t.Fatal(err)
	}
	if has, _ := r.HasRemoteBranch(ctx, "feature"); has {
		t.Fatal("feature not pruned with --prune")
	}
}

func TestBranchChecks(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	r := open(t, f.clone)
	cases := []struct {
		fn   func(context.Context, string) (bool, error)
		name string
		want bool
	}{
		{r.HasBranch, "main", true},
		{r.HasBranch, "feature", false},
		{r.HasBranch, "missing", false},
		{r.HasRemoteBranch, "main", true},
		{r.HasRemoteBranch, "feature", true},
		{r.HasRemoteBranch, "missing", false},
	}
	for _, c := range cases {
		got, err := c.fn(ctx, c.name)
		if err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Errorf("check %q = %v, want %v", c.name, got, c.want)
		}
	}

	git(t, f.clone, "branch", "scratch")
	if err := r.DeleteBranch(ctx, "scratch"); err != nil {
		t.Fatal(err)
	}
	if has, _ := r.HasBranch(ctx, "scratch"); has {
		t.Fatal("scratch still exists")
	}
	if err := r.DeleteBranch(ctx, "scratch"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("DeleteBranch missing = %v, want stderr in error", err)
	}
}

func TestWorktrees(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	r := open(t, f.clone)
	git(t, f.clone, "branch", "existing")

	add := []tmt.AddWorktree{
		{Path: filepath.Join(f.root, "wt-existing"), Branch: "existing", Mode: tmt.WorktreeExisting},
		{Path: filepath.Join(f.root, "wt-track"), Branch: "feature", Start: "origin/feature", Mode: tmt.WorktreeTrackRemote},
		{Path: filepath.Join(f.root, "wt-new"), Branch: "fresh", Start: "origin/main", Mode: tmt.WorktreeNewBranch},
	}
	for _, a := range add {
		if err := r.AddWorktree(ctx, a); err != nil {
			t.Fatalf("AddWorktree %v: %v", a.Mode, err)
		}
	}
	if up := git(t, f.clone, "rev-parse", "--abbrev-ref", "feature@{upstream}"); up != "origin/feature" {
		t.Errorf("feature upstream = %q", up)
	}
	if out, err := exec.Command("git", "-C", f.clone, "rev-parse", "--abbrev-ref", "fresh@{upstream}").CombinedOutput(); err == nil {
		t.Errorf("fresh has upstream %q", out)
	}
	if err := r.AddWorktree(ctx, add[2]); err == nil {
		t.Fatal("re-adding worktree succeeded")
	}

	detached := filepath.Join(f.root, "wt-detached")
	git(t, f.clone, "worktree", "add", "--quiet", "--detach", detached, "HEAD")

	wts, err := r.Worktrees(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []tmt.Worktree{
		{Path: f.clone, Branch: "main"},
		{Path: detached, Branch: ""},
		{Path: add[0].Path, Branch: "existing"},
		{Path: add[2].Path, Branch: "fresh"},
		{Path: add[1].Path, Branch: "feature"},
	}
	sortWorktrees(wts)
	sortWorktrees(want)
	if !slices.Equal(wts, want) {
		t.Fatalf("Worktrees = %v, want %v", wts, want)
	}

	if err := os.RemoveAll(add[2].Path); err != nil {
		t.Fatal(err)
	}
	if err := r.PruneWorktrees(ctx); err != nil {
		t.Fatal(err)
	}
	wts, err = r.Worktrees(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range wts {
		if w.Path == add[2].Path {
			t.Fatalf("pruned worktree still listed: %v", wts)
		}
	}
	if len(wts) != 4 {
		t.Fatalf("Worktrees after prune = %v", wts)
	}
}

func sortWorktrees(w []tmt.Worktree) {
	slices.SortFunc(w, func(a, b tmt.Worktree) int { return strings.Compare(a.Path, b.Path) })
}

func TestParseWorktreesBare(t *testing.T) {
	out := "worktree /r/bare.git\nbare\n\nworktree /r/wt\nHEAD abc\nbranch refs/heads/u/x\n\nworktree /r/d\nHEAD abc\ndetached\n\n"
	got := parseWorktrees(out)
	want := []tmt.Worktree{{Path: "/r/bare.git"}, {Path: "/r/wt", Branch: "u/x"}, {Path: "/r/d"}}
	if !slices.Equal(got, want) {
		t.Fatalf("parseWorktrees = %v, want %v", got, want)
	}
}

func TestStatus(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	r := open(t, f.clone)
	st, err := r.Status(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if st != (tmt.Status{Branch: "main"}) {
		t.Fatalf("clean Status = %+v", st)
	}

	if err := os.WriteFile(filepath.Join(f.clone, "a"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"u1", "u2"} {
		if err := os.WriteFile(filepath.Join(f.clone, n), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if st, err = r.Status(ctx, true); err != nil || st.Dirty != 3 {
		t.Fatalf("Status(untracked) = %+v, %v; want 3 dirty", st, err)
	}
	if st, err = r.Status(ctx, false); err != nil || st.Dirty != 1 {
		t.Fatalf("Status(tracked) = %+v, %v; want 1 dirty", st, err)
	}

	git(t, f.clone, "checkout", "--quiet", "--detach")
	if st, err = r.Status(ctx, false); err != nil || st.Branch != "" {
		t.Fatalf("detached Status = %+v, %v", st, err)
	}
}

func TestFastForwardCheckedOut(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	r := open(t, f.clone)
	pushMain(t, f, "c")
	if err := r.Fetch(ctx, false); err != nil {
		t.Fatal(err)
	}
	if err := r.FastForward(ctx, "main", "origin/main"); err != nil {
		t.Fatal(err)
	}
	if got, want := git(t, f.clone, "rev-parse", "HEAD"), git(t, f.seed, "rev-parse", "main"); got != want {
		t.Fatalf("HEAD = %s, want %s", got, want)
	}
	if _, err := os.Stat(filepath.Join(f.clone, "c")); err != nil {
		t.Fatalf("working tree not updated: %v", err)
	}
}

func TestFastForwardNotCheckedOut(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	r := open(t, f.clone)
	git(t, f.clone, "checkout", "--quiet", "-b", "other")
	pushMain(t, f, "c")
	if err := r.Fetch(ctx, false); err != nil {
		t.Fatal(err)
	}
	if err := r.FastForward(ctx, "main", "origin/main"); err != nil {
		t.Fatal(err)
	}
	if got, want := git(t, f.clone, "rev-parse", "main"), git(t, f.seed, "rev-parse", "main"); got != want {
		t.Fatalf("main = %s, want %s", got, want)
	}
	if cur := git(t, f.clone, "branch", "--show-current"); cur != "other" {
		t.Fatalf("current branch = %q", cur)
	}
}

func TestFastForwardDiverged(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	r := open(t, f.clone)
	pushMain(t, f, "c")
	commit(t, f.clone, "local")
	if err := r.Fetch(ctx, false); err != nil {
		t.Fatal(err)
	}
	if err := r.FastForward(ctx, "main", "origin/main"); err == nil {
		t.Fatal("checked-out diverged FastForward succeeded")
	}
	git(t, f.clone, "checkout", "--quiet", "-b", "other")
	if err := r.FastForward(ctx, "main", "origin/main"); err == nil {
		t.Fatal("not-checked-out diverged FastForward succeeded")
	}
}

func TestRevParseAndCount(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	r := open(t, f.clone)
	sha, ok, err := r.RevParse(ctx, "refs/heads/missing")
	if err != nil || ok || sha != "" {
		t.Fatalf("RevParse missing = %q, %v, %v", sha, ok, err)
	}
	sha, ok, err = r.RevParse(ctx, "main")
	if err != nil || !ok || sha != git(t, f.clone, "rev-parse", "main") {
		t.Fatalf("RevParse main = %q, %v, %v", sha, ok, err)
	}

	commit(t, f.clone, "x")
	commit(t, f.clone, "y")
	n, err := r.CountCommits(ctx, "origin/main", "HEAD")
	if err != nil || n != 2 {
		t.Fatalf("CountCommits = %d, %v; want 2", n, err)
	}
	if n, err = r.CountCommits(ctx, "HEAD", "origin/main"); err != nil || n != 0 {
		t.Fatalf("CountCommits reversed = %d, %v; want 0", n, err)
	}
	if _, err = r.CountCommits(ctx, "nope", "HEAD"); err == nil {
		t.Fatal("CountCommits with bad ref succeeded")
	}
}

func TestBranchesContaining(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	r := open(t, f.clone)
	git(t, f.clone, "branch", "same")
	git(t, f.clone, "checkout", "--quiet", "-b", "ahead")
	commit(t, f.clone, "x")
	git(t, f.clone, "checkout", "--quiet", "main")

	got, err := r.BranchesContaining(ctx, "main")
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(got)
	if want := []string{"ahead", "main", "same"}; !slices.Equal(got, want) {
		t.Fatalf("BranchesContaining main = %v, want %v", got, want)
	}
	if got, err = r.BranchesContaining(ctx, "ahead"); err != nil || !slices.Equal(got, []string{"ahead"}) {
		t.Fatalf("BranchesContaining ahead = %v, %v", got, err)
	}
}

func TestOpener(t *testing.T) {
	f := setup(t)
	o := New()
	sub := filepath.Join(f.clone, "sub", "dir")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	r, ok, err := o.Find(context.Background(), sub)
	if err != nil || !ok || r.Path() != f.clone {
		t.Fatalf("Find inside = %v, %v, %v", r, ok, err)
	}
	outside := filepath.Join(f.root, "plain")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if r, ok, err = o.Find(context.Background(), outside); err != nil || ok || r != nil {
		t.Fatalf("Find outside = %v, %v, %v", r, ok, err)
	}

	wt := filepath.Join(f.root, "wt")
	git(t, f.clone, "worktree", "add", "--quiet", "-b", "w", wt)
	stale := filepath.Join(f.root, "stale")
	git(t, f.clone, "worktree", "add", "--quiet", "-b", "s", stale)
	if err := os.RemoveAll(filepath.Join(f.clone, ".git", "worktrees", "stale")); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]bool{f.clone: true, wt: true, stale: false, sub: false, outside: false} {
		if got := o.IsRepo(path); got != want {
			t.Errorf("IsRepo(%s) = %v, want %v", path, got, want)
		}
	}

	if r, err = o.Open(f.clone); err != nil || r.Path() != f.clone {
		t.Fatalf("Open = %v, %v", r, err)
	}
}

func TestRender(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	r := open(t, f.clone)
	st, err := r.RenderStatus(ctx)
	if err != nil || !strings.Contains(st, "main") {
		t.Fatalf("RenderStatus = %q, %v", st, err)
	}
	log, err := r.RenderLog(ctx, 5)
	if err != nil || !strings.Contains(log, "a") || !strings.Contains(log, "\x1b[") {
		t.Fatalf("RenderLog = %q, %v", log, err)
	}
}

func TestInjectableBinary(t *testing.T) {
	hermetic(t)
	o := &Opener{Bin: "/nonexistent/git"}
	r, _ := o.Open(t.TempDir())
	if _, err := r.RenderStatus(context.Background()); err == nil {
		t.Fatal("expected error from missing binary")
	}
	if _, _, err := o.Find(context.Background(), t.TempDir()); err == nil {
		t.Fatal("Find with missing binary should error")
	}
}
