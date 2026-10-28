package gitcli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

var (
	_ tmt.RepoOpener = (*Opener)(nil)
	_ tmt.Repo       = (*Repo)(nil)
)

type Opener struct {
	Bin string
}

func New() *Opener {
	return &Opener{Bin: "git"}
}

func (o *Opener) bin() string {
	if o.Bin == "" {
		return "git"
	}
	return o.Bin
}

func (o *Opener) Open(path string) (tmt.Repo, error) {
	return &Repo{Bin: o.bin(), Dir: path}, nil
}

func (o *Opener) Find(ctx context.Context, path string) (tmt.Repo, bool, error) {
	r := &Repo{Bin: o.bin(), Dir: path}
	top, err := r.line(ctx, "rev-parse", "--show-toplevel")
	if _, ok := errors.AsType[*exitError](err); ok {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &Repo{Bin: o.bin(), Dir: top}, true, nil
}

func (o *Opener) IsRepo(path string) bool {
	dotgit := filepath.Join(path, ".git")
	fi, err := os.Stat(dotgit)
	if err != nil {
		return false
	}
	if fi.IsDir() {
		return true
	}
	data, err := os.ReadFile(dotgit)
	if err != nil {
		return false
	}
	gitdir, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir:")
	if !ok {
		return false
	}
	gitdir = strings.TrimSpace(gitdir)
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(path, gitdir)
	}
	fi, err = os.Stat(gitdir)
	return err == nil && fi.IsDir()
}

type Repo struct {
	Bin string
	Dir string
}

type exitError struct {
	args   []string
	code   int
	stderr string
}

func (e *exitError) Error() string {
	msg := fmt.Sprintf("git %s: exit %d", strings.Join(e.args, " "), e.code)
	if e.stderr != "" {
		msg += ": " + e.stderr
	}
	return msg
}

func (r *Repo) run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, r.Bin, append([]string{"-C", r.Dir}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ctx.Err() == nil {
			return "", &exitError{args: args, code: ee.ExitCode(), stderr: strings.TrimSpace(stderr.String())}
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return stdout.String(), nil
}

func (r *Repo) line(ctx context.Context, args ...string) (string, error) {
	out, err := r.run(ctx, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (r *Repo) check(ctx context.Context, args ...string) (bool, error) {
	_, err := r.run(ctx, args...)
	var ee *exitError
	if errors.As(err, &ee) && ee.code == 1 {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func lines(out string) []string {
	var res []string
	for l := range strings.SplitSeq(out, "\n") {
		if strings.TrimSpace(l) != "" {
			res = append(res, strings.TrimSpace(l))
		}
	}
	return res
}

func (r *Repo) Path() string {
	return r.Dir
}

func (r *Repo) RemoteURL(ctx context.Context, remote string) (string, error) {
	return r.line(ctx, "remote", "get-url", remote)
}

func (r *Repo) Fetch(ctx context.Context, prune bool) error {
	args := []string{"fetch", "--quiet"}
	if prune {
		args = append(args, "--prune")
	}
	_, err := r.run(ctx, append(args, "origin")...)
	return err
}

func (r *Repo) DefaultRef(ctx context.Context) (string, error) {
	for _, ref := range []string{"origin/HEAD", "origin/main", "origin/master"} {
		_, ok, err := r.RevParse(ctx, ref)
		if err != nil {
			return "", err
		}
		if ok {
			return r.line(ctx, "rev-parse", "--abbrev-ref", ref)
		}
	}
	return "HEAD", nil
}

func (r *Repo) HasBranch(ctx context.Context, branch string) (bool, error) {
	return r.check(ctx, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
}

func (r *Repo) HasRemoteBranch(ctx context.Context, branch string) (bool, error) {
	return r.check(ctx, "show-ref", "--verify", "--quiet", "refs/remotes/origin/"+branch)
}

func (r *Repo) DeleteBranch(ctx context.Context, branch string) error {
	_, err := r.run(ctx, "branch", "-D", branch)
	return err
}

func (r *Repo) BranchesContaining(ctx context.Context, ref string) ([]string, error) {
	out, err := r.run(ctx, "for-each-ref", "--contains", ref, "--format=%(refname:short)", "refs/heads")
	if err != nil {
		return nil, err
	}
	return lines(out), nil
}

func (r *Repo) FastForward(ctx context.Context, branch, upstream string) error {
	current, err := r.line(ctx, "branch", "--show-current")
	if err != nil {
		return err
	}
	if current == branch {
		_, err = r.run(ctx, "merge", "--quiet", "--ff-only", upstream)
		return err
	}
	_, err = r.run(ctx, "fetch", "--quiet", ".", upstream+":"+branch)
	return err
}

func (r *Repo) RevParse(ctx context.Context, ref string) (string, bool, error) {
	out, err := r.line(ctx, "rev-parse", "--verify", "--quiet", ref)
	var ee *exitError
	if errors.As(err, &ee) && ee.code == 1 {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return out, true, nil
}

func (r *Repo) CountCommits(ctx context.Context, from, to string) (int, error) {
	out, err := r.line(ctx, "rev-list", "--count", from+".."+to)
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(out)
	if err != nil {
		return 0, fmt.Errorf("git rev-list --count %s..%s: unexpected output %q", from, to, out)
	}
	return n, nil
}

func (r *Repo) Branch(ctx context.Context) (string, error) {
	return r.line(ctx, "branch", "--show-current")
}

func (r *Repo) Status(ctx context.Context, untracked bool) (tmt.Status, error) {
	branch, err := r.Branch(ctx)
	if err != nil {
		return tmt.Status{}, err
	}
	args := []string{"status", "--porcelain"}
	if !untracked {
		args = append(args, "--untracked-files=no")
	}
	out, err := r.run(ctx, args...)
	if err != nil {
		return tmt.Status{}, err
	}
	return tmt.Status{Branch: branch, Dirty: len(lines(out))}, nil
}

func (r *Repo) AddWorktree(ctx context.Context, opts tmt.AddWorktree) error {
	args := []string{"worktree", "add", "--quiet"}
	switch opts.Mode {
	case tmt.WorktreeExisting:
		args = append(args, opts.Path, opts.Branch)
	case tmt.WorktreeTrackRemote:
		args = append(args, "--track", "-b", opts.Branch, opts.Path, opts.Start)
	case tmt.WorktreeNewBranch:
		args = append(args, "--no-track", "-b", opts.Branch, opts.Path, opts.Start)
	default:
		return fmt.Errorf("unknown worktree mode %d", opts.Mode)
	}
	_, err := r.run(ctx, args...)
	return err
}

func (r *Repo) Worktrees(ctx context.Context) ([]tmt.Worktree, error) {
	out, err := r.run(ctx, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	return parseWorktrees(out), nil
}

func parseWorktrees(out string) []tmt.Worktree {
	var wts []tmt.Worktree
	for l := range strings.SplitSeq(out, "\n") {
		switch {
		case strings.HasPrefix(l, "worktree "):
			wts = append(wts, tmt.Worktree{Path: strings.TrimPrefix(l, "worktree ")})
		case strings.HasPrefix(l, "branch ") && len(wts) > 0:
			wts[len(wts)-1].Branch = strings.TrimPrefix(strings.TrimPrefix(l, "branch "), "refs/heads/")
		}
	}
	return wts
}

func (r *Repo) PruneWorktrees(ctx context.Context) error {
	_, err := r.run(ctx, "worktree", "prune")
	return err
}

func (r *Repo) RenderStatus(ctx context.Context) (string, error) {
	return r.run(ctx, "-c", "color.status=always", "status", "-sb")
}

func (r *Repo) RenderLog(ctx context.Context, n int) (string, error) {
	return r.run(ctx, "log", "--color=always", "--graph", "--oneline", "--decorate", "-n", strconv.Itoa(n))
}
