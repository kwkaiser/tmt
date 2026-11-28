package tmt_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"git.kwkaiser.io/kwkaiser/tmt/internal/gitcli"
	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func commitFile(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", name)
	runGit(t, dir, "commit", "--quiet", "-m", name)
}

func localRepoEnv(t *testing.T) (*env, tmt.Task, string) {
	t.Helper()
	e := newEnv(t)
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(e.base))
	t.Setenv("GIT_AUTHOR_NAME", "tmt")
	t.Setenv("GIT_AUTHOR_EMAIL", "tmt@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "tmt")
	t.Setenv("GIT_COMMITTER_EMAIL", "tmt@example.com")
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "init.defaultBranch")
	t.Setenv("GIT_CONFIG_VALUE_0", "main")

	src := filepath.Join(e.base, "api")
	runGit(t, e.base, "init", "--quiet", src)
	commitFile(t, src, "a")
	e.app.Git = gitcli.New()

	ctx := context.Background()
	if err := e.app.New(ctx, tmt.NewOptions{Base: e.base, Topic: "local", Repos: []string{"api"}, Detach: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.app.Fill(ctx, "work-local", []string{"api"}); err != nil {
		t.Fatal(err)
	}
	task := e.task(t, "work-local")
	wt := filepath.Join(task.Dir, "api")
	commitFile(t, wt, "b")
	commitFile(t, wt, "c")
	return e, task, wt
}

func TestLocalOnlyRepoCountsCommits(t *testing.T) {
	e, _, _ := localRepoEnv(t)
	e.out.Reset()
	if err := e.app.Ls(context.Background(), tmt.LsOptions{JSON: true}); err != nil {
		t.Fatal(err)
	}
	var got []struct {
		Repos []struct {
			Name     string
			Commits  int
			Unpushed int
		}
	}
	if err := json.Unmarshal(e.out.Bytes(), &got); err != nil {
		t.Fatalf("%v\n%s", err, e.out.String())
	}
	if len(got) != 1 || len(got[0].Repos) != 1 {
		t.Fatalf("ls = %s", e.out.String())
	}
	if r := got[0].Repos[0]; r.Name != "api" || r.Commits != 2 || r.Unpushed != 2 {
		t.Fatalf("repo = %+v; want api with commits 2, unpushed 2", r)
	}
}

func TestLocalOnlyRepoDoneRefusesUnpushed(t *testing.T) {
	e, task, wt := localRepoEnv(t)
	ctx := context.Background()
	if err := e.app.Done(ctx, tmt.DoneOptions{Topic: task.Topic, Yes: true}); !errors.Is(err, tmt.ErrUnsafe) {
		t.Fatalf("Done = %v; want ErrUnsafe", err)
	}
	if _, err := os.Stat(wt); err != nil {
		t.Fatalf("worktree removed: %v", err)
	}
	runGit(t, filepath.Join(e.base, "api"), "show-ref", "--verify", "--quiet", "refs/heads/"+task.Branch)
}
