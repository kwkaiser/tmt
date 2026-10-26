package tmt

import "context"

type PRState string

const (
	PRNone   PRState = "NONE"
	PROpen   PRState = "OPEN"
	PRClosed PRState = "CLOSED"
	PRMerged PRState = "MERGED"
)

type MergedPR struct {
	Branch  string
	HeadSHA string
}

type Forge interface {
	PRStates(ctx context.Context, repo Repo, branches []string) (map[string]PRState, error)
	MergedBranches(ctx context.Context, repo Repo) ([]MergedPR, error)
}

type ForgeResolver interface {
	For(ctx context.Context, repo Repo) (Forge, error)
}
