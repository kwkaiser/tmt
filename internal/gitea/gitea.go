package gitea

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"sync"

	"code.gitea.io/sdk/gitea"
	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

const (
	pageSize = 50
	maxPages = 4
)

type pull struct {
	Branch string
	SHA    string
	State  tmt.PRState
}

type listing struct {
	once  sync.Once
	pulls []pull
	err   error
}

type Forge struct {
	Getenv      func(string) string
	ConfigPaths []string
	HTTPClient  *http.Client

	mu    sync.Mutex
	cache map[string]*listing
}

func New() *Forge {
	return &Forge{}
}

func (f *Forge) PRStates(ctx context.Context, repo tmt.Repo, branches []string) (map[string]tmt.PRState, error) {
	pulls, err := f.pulls(ctx, repo)
	if err != nil {
		return nil, err
	}
	return pickStates(pulls, branches), nil
}

func (f *Forge) MergedBranches(ctx context.Context, repo tmt.Repo) ([]tmt.MergedPR, error) {
	pulls, err := f.pulls(ctx, repo)
	if err != nil {
		return nil, err
	}
	return mergedPulls(pulls), nil
}

func (f *Forge) pulls(ctx context.Context, repo tmt.Repo) ([]pull, error) {
	key := repo.Path()
	f.mu.Lock()
	if f.cache == nil {
		f.cache = map[string]*listing{}
	}
	l, ok := f.cache[key]
	if !ok {
		l = &listing{}
		f.cache[key] = l
	}
	f.mu.Unlock()
	l.once.Do(func() {
		l.pulls, l.err = f.list(ctx, repo)
	})
	return l.pulls, l.err
}

func (f *Forge) list(ctx context.Context, repo tmt.Repo) ([]pull, error) {
	url, err := repo.RemoteURL(ctx, "origin")
	if err != nil {
		return nil, err
	}
	r, err := parseRemote(url)
	if err != nil {
		return nil, err
	}
	client, err := f.client(ctx, r.Host)
	if err != nil {
		return nil, err
	}
	var all []pull
	for page := 1; page <= maxPages; page++ {
		prs, _, err := client.ListRepoPullRequests(r.Owner, r.Name, listOptions(page))
		if err != nil {
			return nil, fmt.Errorf("gitea list pulls %s/%s: %w", r.Owner, r.Name, err)
		}
		all = append(all, mapPulls(prs)...)
		if len(prs) < pageSize {
			break
		}
	}
	return all, nil
}

func listOptions(page int) gitea.ListPullRequestsOptions {
	return gitea.ListPullRequestsOptions{
		ListOptions: gitea.ListOptions{Page: page, PageSize: pageSize},
		State:       gitea.StateAll,
	}
}

func (f *Forge) client(ctx context.Context, host string) (*gitea.Client, error) {
	l, err := f.login(host)
	if err != nil {
		return nil, err
	}
	opts := []gitea.ClientOption{
		gitea.SetContext(ctx),
		gitea.SetGiteaVersion(""),
		gitea.SetHTTPClient(f.httpClient(l.Insecure)),
	}
	if l.Token != "" {
		opts = append(opts, gitea.SetToken(l.Token))
	}
	client, err := gitea.NewClient(l.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("gitea client for %s: %w", host, err)
	}
	return client, nil
}

func (f *Forge) login(host string) (login, error) {
	getenv := f.getenv()
	if l, ok := envLogin(getenv, host); ok {
		return l, nil
	}
	paths := f.ConfigPaths
	if paths == nil {
		paths = teaConfigPaths(getenv)
	}
	cfg, path, err := readTeaConfig(paths)
	if err != nil {
		return login{}, err
	}
	l, ok := cfg.loginFor(host)
	if !ok {
		return login{}, fmt.Errorf("no tea login for %s in %s", host, path)
	}
	return l, nil
}

func (f *Forge) getenv() func(string) string {
	if f.Getenv != nil {
		return f.Getenv
	}
	return os.Getenv
}

func (f *Forge) httpClient(insecure bool) *http.Client {
	if f.HTTPClient != nil {
		return f.HTTPClient
	}
	if !insecure {
		return &http.Client{}
	}
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	return &http.Client{Transport: t}
}

func mapPulls(prs []*gitea.PullRequest) []pull {
	pulls := make([]pull, 0, len(prs))
	for _, p := range prs {
		if p == nil || p.Head == nil {
			continue
		}
		pulls = append(pulls, pull{Branch: headBranch(p.Head), SHA: p.Head.Sha, State: pullState(p.HasMerged, p.State)})
	}
	return pulls
}

func headBranch(h *gitea.PRBranchInfo) string {
	if h.Name != "" {
		return h.Name
	}
	return h.Ref
}

func pullState(merged bool, state gitea.StateType) tmt.PRState {
	switch {
	case merged:
		return tmt.PRMerged
	case state == gitea.StateClosed:
		return tmt.PRClosed
	default:
		return tmt.PROpen
	}
}

func pickStates(pulls []pull, branches []string) map[string]tmt.PRState {
	states := make(map[string]tmt.PRState, len(branches))
	for _, b := range branches {
		states[b] = tmt.PRNone
	}
	seen := map[string]bool{}
	for _, p := range pulls {
		if seen[p.Branch] {
			continue
		}
		seen[p.Branch] = true
		if _, ok := states[p.Branch]; ok {
			states[p.Branch] = p.State
		}
	}
	return states
}

func mergedPulls(pulls []pull) []tmt.MergedPR {
	var merged []tmt.MergedPR
	for _, p := range pulls {
		if p.State == tmt.PRMerged {
			merged = append(merged, tmt.MergedPR{Branch: p.Branch, HeadSHA: p.SHA})
		}
	}
	return merged
}
