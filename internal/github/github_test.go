package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sync"
	"testing"

	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

type fakeRepo struct {
	tmt.Repo
	url string
}

func (r fakeRepo) RemoteURL(context.Context, string) (string, error) { return r.url, nil }

type gqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

type fakeAPI struct {
	t       *testing.T
	mu      sync.Mutex
	reqs    []gqlRequest
	hosts   []string
	auth    []string
	respond func(gqlRequest) any
}

func (a *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req gqlRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.t.Errorf("decode: %v", err)
	}
	a.mu.Lock()
	a.reqs = append(a.reqs, req)
	a.hosts = append(a.hosts, r.Host+r.URL.Path)
	a.auth = append(a.auth, r.Header.Get("Authorization"))
	a.mu.Unlock()
	_ = json.NewEncoder(w).Encode(map[string]any{"data": a.respond(req)})
}

type rewrite struct {
	target *url.URL
}

func (rw rewrite) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Host = r.URL.Host
	r.URL.Scheme = rw.target.Scheme
	r.URL.Host = rw.target.Host
	return http.DefaultTransport.RoundTrip(r)
}

func newForge(t *testing.T, respond func(gqlRequest) any) (*Forge, *fakeAPI) {
	t.Helper()
	t.Setenv("GH_CONFIG_DIR", t.TempDir())
	api := &fakeAPI{t: t, respond: respond}
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)
	target, _ := url.Parse(srv.URL)
	return &Forge{Token: "test-token", Transport: rewrite{target: target}}, api
}

func TestParseRemote(t *testing.T) {
	cases := map[string][3]string{
		"git@github.com:owner/repo.git":       {"github.com", "owner", "repo"},
		"https://github.com/owner/repo":       {"github.com", "owner", "repo"},
		"https://github.com/owner/repo.git\n": {"github.com", "owner", "repo"},
		"ssh://git@github.com/owner/repo.git": {"github.com", "owner", "repo"},
	}
	for in, want := range cases {
		r, err := parseRemote(in)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if got := [3]string{r.Host, r.Owner, r.Name}; got != want {
			t.Errorf("%q: got %v want %v", in, got, want)
		}
	}
	for _, in := range []string{"", "not a url"} {
		if _, err := parseRemote(in); err == nil {
			t.Errorf("%q: expected error", in)
		}
	}
}

func TestStatesQuery(t *testing.T) {
	got := statesQuery(2)
	want := "query($owner:String!,$repo:String!,$h0:String!,$h1:String!){repository(owner:$owner,name:$repo){" +
		"b0:pullRequests(headRefName:$h0,first:1,orderBy:{field:CREATED_AT,direction:DESC}){nodes{state}} " +
		"b1:pullRequests(headRefName:$h1,first:1,orderBy:{field:CREATED_AT,direction:DESC}){nodes{state}} }}"
	if got != want {
		t.Fatalf("query mismatch\ngot:  %s\nwant: %s", got, want)
	}
}

func TestPRStates(t *testing.T) {
	f, api := newForge(t, func(gqlRequest) any {
		return map[string]any{"repository": map[string]any{
			"b0": map[string]any{"nodes": []any{map[string]any{"state": "MERGED"}}},
			"b1": map[string]any{"nodes": []any{}},
			"b2": map[string]any{"nodes": []any{map[string]any{"state": "OPEN"}}},
			"b9": map[string]any{"nodes": []any{map[string]any{"state": "CLOSED"}}},
		}}
	})
	got, err := f.PRStates(context.Background(), fakeRepo{url: "git@github.com:o/r.git"}, []string{"one", "a/two", "three"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]tmt.PRState{"one": tmt.PRMerged, "a/two": tmt.PRNone, "three": tmt.PROpen}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if len(api.reqs) != 1 {
		t.Fatalf("requests %d want 1", len(api.reqs))
	}
	req := api.reqs[0]
	if req.Query != statesQuery(3) {
		t.Fatalf("query %q", req.Query)
	}
	wantVars := map[string]any{"owner": "o", "repo": "r", "h0": "one", "h1": "a/two", "h2": "three"}
	if !reflect.DeepEqual(req.Variables, wantVars) {
		t.Fatalf("vars %v want %v", req.Variables, wantVars)
	}
	if api.hosts[0] != "api.github.com/graphql" {
		t.Fatalf("endpoint %q", api.hosts[0])
	}
	if api.auth[0] != "token test-token" {
		t.Fatalf("auth header %q", api.auth[0])
	}
}

func TestPRStatesEmptySkipsNetwork(t *testing.T) {
	f, api := newForge(t, func(gqlRequest) any { return nil })
	got, err := f.PRStates(context.Background(), fakeRepo{url: "git@github.com:o/r.git"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 || len(api.reqs) != 0 {
		t.Fatalf("got %v with %d requests", got, len(api.reqs))
	}
}

func TestPRStatesGraphQLError(t *testing.T) {
	t.Setenv("GH_CONFIG_DIR", t.TempDir())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"errors":[{"message":"boom"}]}`))
	}))
	defer srv.Close()
	target, _ := url.Parse(srv.URL)
	f := &Forge{Token: "x", Transport: rewrite{target: target}}
	if _, err := f.PRStates(context.Background(), fakeRepo{url: "https://github.com/o/r"}, []string{"a"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestMergedBranchesPaginates(t *testing.T) {
	f, api := newForge(t, func(req gqlRequest) any {
		first := int(req.Variables["first"].(float64))
		after, _ := req.Variables["after"].(string)
		page := 0
		if after != "" {
			fmt.Sscanf(after, "c%d", &page)
		}
		nodes := make([]any, 0, first)
		for i := range first {
			n := page*100 + i
			nodes = append(nodes, map[string]any{"headRefName": fmt.Sprintf("b%d", n), "headRefOid": fmt.Sprintf("s%d", n)})
		}
		return map[string]any{"search": map[string]any{
			"pageInfo": map[string]any{"hasNextPage": true, "endCursor": fmt.Sprintf("c%d", page+1)},
			"nodes":    nodes,
		}}
	})
	got, err := f.MergedBranches(context.Background(), fakeRepo{url: "https://github.com/o/r.git"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != mergedLimit {
		t.Fatalf("got %d want %d", len(got), mergedLimit)
	}
	if got[0] != (tmt.MergedPR{Branch: "b0", HeadSHA: "s0"}) || got[199] != (tmt.MergedPR{Branch: "b199", HeadSHA: "s199"}) {
		t.Fatalf("unexpected bounds %v %v", got[0], got[199])
	}
	if len(api.reqs) != 2 {
		t.Fatalf("requests %d want 2", len(api.reqs))
	}
	if q := api.reqs[0].Variables["q"]; q != "repo:o/r is:pr is:merged author:@me sort:created-desc" {
		t.Fatalf("search %q", q)
	}
	if api.reqs[0].Variables["after"] != nil || api.reqs[1].Variables["after"] != "c1" {
		t.Fatalf("cursors %v %v", api.reqs[0].Variables["after"], api.reqs[1].Variables["after"])
	}
}

func TestMergedBranchesStopsOnLastPage(t *testing.T) {
	f, api := newForge(t, func(gqlRequest) any {
		return map[string]any{"search": map[string]any{
			"pageInfo": map[string]any{"hasNextPage": false, "endCursor": "c1"},
			"nodes": []any{
				map[string]any{"headRefName": "feat", "headRefOid": "abc"},
				map[string]any{},
				map[string]any{"headRefName": "fix", "headRefOid": "def"},
			},
		}}
	})
	got, err := f.MergedBranches(context.Background(), fakeRepo{url: "git@github.com:o/r.git"})
	if err != nil {
		t.Fatal(err)
	}
	want := []tmt.MergedPR{{Branch: "feat", HeadSHA: "abc"}, {Branch: "fix", HeadSHA: "def"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if len(api.reqs) != 1 {
		t.Fatalf("requests %d want 1", len(api.reqs))
	}
}

func TestClientReused(t *testing.T) {
	f, _ := newForge(t, func(gqlRequest) any { return nil })
	a, err := f.client("github.com")
	if err != nil {
		t.Fatal(err)
	}
	b, err := f.client("github.com")
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatal("expected cached client")
	}
}
