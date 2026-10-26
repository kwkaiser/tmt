package forge

import (
	"context"
	"regexp"

	"git.kwkaiser.io/kwkaiser/tmt/internal/gitea"
	"git.kwkaiser.io/kwkaiser/tmt/internal/github"
	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

var githubURL = regexp.MustCompile(`github\.com[:/]`)

type Resolver struct {
	GitHub tmt.Forge
	Gitea  tmt.Forge
}

func NewResolver() *Resolver {
	return &Resolver{GitHub: github.New(), Gitea: gitea.New()}
}

func (r *Resolver) For(ctx context.Context, repo tmt.Repo) (tmt.Forge, error) {
	url, err := repo.RemoteURL(ctx, "origin")
	if err == nil && isGitHub(url) {
		return r.GitHub, nil
	}
	return r.Gitea, nil
}

func isGitHub(url string) bool {
	return githubURL.MatchString(url)
}
