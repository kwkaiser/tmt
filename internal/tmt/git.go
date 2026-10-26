package tmt

import "context"

type Worktree struct {
	Path   string
	Branch string
}

type WorktreeMode int

const (
	WorktreeExisting WorktreeMode = iota
	WorktreeTrackRemote
	WorktreeNewBranch
)

type AddWorktree struct {
	Path   string
	Branch string
	Start  string
	Mode   WorktreeMode
}

type Status struct {
	Branch string
	Dirty  int
}

type Repo interface {
	Path() string
	RemoteURL(ctx context.Context, remote string) (string, error)
	Fetch(ctx context.Context, prune bool) error
	DefaultRef(ctx context.Context) (string, error)

	HasBranch(ctx context.Context, branch string) (bool, error)
	HasRemoteBranch(ctx context.Context, branch string) (bool, error)
	DeleteBranch(ctx context.Context, branch string) error
	BranchesContaining(ctx context.Context, ref string) ([]string, error)
	FastForward(ctx context.Context, branch, upstream string) error

	RevParse(ctx context.Context, ref string) (string, bool, error)
	CountCommits(ctx context.Context, from, to string) (int, error)
	Status(ctx context.Context, untracked bool) (Status, error)

	AddWorktree(ctx context.Context, opts AddWorktree) error
	Worktrees(ctx context.Context) ([]Worktree, error)
	PruneWorktrees(ctx context.Context) error

	RenderStatus(ctx context.Context) (string, error)
	RenderLog(ctx context.Context, n int) (string, error)
}

type RepoOpener interface {
	Open(path string) (Repo, error)
	Find(path string) (Repo, bool, error)
	IsRepo(path string) bool
}
