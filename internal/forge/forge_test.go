package forge

import (
	"context"
	"errors"
	"testing"

	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

type fakeRepo struct {
	tmt.Repo
	url string
	err error
}

func (r fakeRepo) RemoteURL(context.Context, string) (string, error) { return r.url, r.err }

type stubForge struct {
	tmt.Forge
	name string
}

func TestIsGitHub(t *testing.T) {
	cases := map[string]bool{
		"git@github.com:owner/repo.git":         true,
		"https://github.com/owner/repo":         true,
		"ssh://git@github.com/owner/repo.git":   true,
		"https://git.kwkaiser.io/owner/repo":    false,
		"git@git.kwkaiser.io:owner/repo.git":    false,
		"https://github.company.com/owner/repo": false,
		"":                                      false,
	}
	for url, want := range cases {
		if got := isGitHub(url); got != want {
			t.Errorf("isGitHub(%q) = %v want %v", url, got, want)
		}
	}
}

func TestResolverFor(t *testing.T) {
	gh := &stubForge{name: "github"}
	gt := &stubForge{name: "gitea"}
	r := &Resolver{GitHub: gh, Gitea: gt}
	cases := []struct {
		repo fakeRepo
		want tmt.Forge
	}{
		{fakeRepo{url: "git@github.com:o/r.git"}, gh},
		{fakeRepo{url: "https://git.kwkaiser.io/o/r"}, gt},
		{fakeRepo{err: errors.New("no origin")}, gt},
	}
	for _, c := range cases {
		got, err := r.For(context.Background(), c.repo)
		if err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Errorf("For(%q) = %v want %v", c.repo.url, got, c.want)
		}
	}
}
