package github

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/repository"
)

const (
	mergedLimit = 200
	searchPage  = 100
)

type Forge struct {
	Token     string
	Transport http.RoundTripper

	mu      sync.Mutex
	clients map[string]*api.GraphQLClient
}

func New() *Forge {
	return &Forge{}
}

func (f *Forge) PRStates(ctx context.Context, repo tmt.Repo, branches []string) (map[string]tmt.PRState, error) {
	if len(branches) == 0 {
		return map[string]tmt.PRState{}, nil
	}
	r, client, err := f.resolve(ctx, repo)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Repository map[string]stateNodes `json:"repository"`
	}
	if err := client.DoWithContext(ctx, statesQuery(len(branches)), statesVars(r, branches), &resp); err != nil {
		return nil, fmt.Errorf("github pr states: %w", err)
	}
	return mapStates(resp.Repository, branches), nil
}

func (f *Forge) MergedBranches(ctx context.Context, repo tmt.Repo) ([]tmt.MergedPR, error) {
	r, client, err := f.resolve(ctx, repo)
	if err != nil {
		return nil, err
	}
	var merged []tmt.MergedPR
	var cursor *string
	for len(merged) < mergedLimit {
		page, err := searchMerged(ctx, client, r, min(searchPage, mergedLimit-len(merged)), cursor)
		if err != nil {
			return nil, err
		}
		merged = append(merged, page.prs()...)
		if !page.PageInfo.HasNextPage || page.PageInfo.EndCursor == "" {
			break
		}
		cursor = &page.PageInfo.EndCursor
	}
	return merged, nil
}

func (f *Forge) resolve(ctx context.Context, repo tmt.Repo) (repository.Repository, *api.GraphQLClient, error) {
	url, err := repo.RemoteURL(ctx, "origin")
	if err != nil {
		return repository.Repository{}, nil, err
	}
	r, err := parseRemote(url)
	if err != nil {
		return repository.Repository{}, nil, err
	}
	client, err := f.client(r.Host)
	if err != nil {
		return repository.Repository{}, nil, err
	}
	return r, client, nil
}

func parseRemote(url string) (repository.Repository, error) {
	r, err := repository.Parse(strings.TrimSpace(url))
	if err != nil {
		return repository.Repository{}, fmt.Errorf("parse github remote %q: %w", url, err)
	}
	if r.Host == "" || r.Owner == "" || r.Name == "" {
		return repository.Repository{}, fmt.Errorf("parse github remote %q: missing host, owner or name", url)
	}
	return r, nil
}

func (f *Forge) client(host string) (*api.GraphQLClient, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c, ok := f.clients[host]; ok {
		return c, nil
	}
	c, err := api.NewGraphQLClient(api.ClientOptions{Host: host, AuthToken: f.Token, Transport: f.Transport})
	if err != nil {
		return nil, fmt.Errorf("github client for %s: %w", host, err)
	}
	if f.clients == nil {
		f.clients = map[string]*api.GraphQLClient{}
	}
	f.clients[host] = c
	return c, nil
}

func statesQuery(n int) string {
	var decl, body strings.Builder
	for i := range n {
		fmt.Fprintf(&decl, ",$h%d:String!", i)
		fmt.Fprintf(&body, "b%d:pullRequests(headRefName:$h%d,first:1,orderBy:{field:CREATED_AT,direction:DESC}){nodes{state}} ", i, i)
	}
	return "query($owner:String!,$repo:String!" + decl.String() + "){repository(owner:$owner,name:$repo){" + body.String() + "}}"
}

func statesVars(r repository.Repository, branches []string) map[string]any {
	vars := map[string]any{"owner": r.Owner, "repo": r.Name}
	for i, b := range branches {
		vars["h"+strconv.Itoa(i)] = b
	}
	return vars
}

type stateNodes struct {
	Nodes []struct {
		State string `json:"state"`
	} `json:"nodes"`
}

func mapStates(repo map[string]stateNodes, branches []string) map[string]tmt.PRState {
	states := make(map[string]tmt.PRState, len(branches))
	for _, b := range branches {
		states[b] = tmt.PRNone
	}
	for key, v := range repo {
		i, err := strconv.Atoi(strings.TrimPrefix(key, "b"))
		if err != nil || i < 0 || i >= len(branches) {
			continue
		}
		states[branches[i]] = nodeState(v)
	}
	return states
}

func nodeState(v stateNodes) tmt.PRState {
	if len(v.Nodes) == 0 || v.Nodes[0].State == "" {
		return tmt.PRNone
	}
	return tmt.PRState(v.Nodes[0].State)
}

const mergedQuery = `query($q:String!,$first:Int!,$after:String){search(type:ISSUE,query:$q,first:$first,after:$after){pageInfo{hasNextPage endCursor} nodes{... on PullRequest{headRefName headRefOid}}}}`

func mergedSearch(r repository.Repository) string {
	return fmt.Sprintf("repo:%s/%s is:pr is:merged author:@me sort:created-desc", r.Owner, r.Name)
}

type searchResult struct {
	PageInfo struct {
		HasNextPage bool   `json:"hasNextPage"`
		EndCursor   string `json:"endCursor"`
	} `json:"pageInfo"`
	Nodes []struct {
		HeadRefName string `json:"headRefName"`
		HeadRefOid  string `json:"headRefOid"`
	} `json:"nodes"`
}

func (s searchResult) prs() []tmt.MergedPR {
	prs := make([]tmt.MergedPR, 0, len(s.Nodes))
	for _, n := range s.Nodes {
		if n.HeadRefName == "" {
			continue
		}
		prs = append(prs, tmt.MergedPR{Branch: n.HeadRefName, HeadSHA: n.HeadRefOid})
	}
	return prs
}

func searchMerged(ctx context.Context, client *api.GraphQLClient, r repository.Repository, first int, after *string) (searchResult, error) {
	vars := map[string]any{"q": mergedSearch(r), "first": first, "after": after}
	var resp struct {
		Search searchResult `json:"search"`
	}
	if err := client.DoWithContext(ctx, mergedQuery, vars, &resp); err != nil {
		return searchResult{}, fmt.Errorf("github merged prs: %w", err)
	}
	return resp.Search, nil
}
