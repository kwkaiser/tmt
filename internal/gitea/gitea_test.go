package gitea

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"sync"
	"testing"

	"code.gitea.io/sdk/gitea"
	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

type fakeRepo struct {
	tmt.Repo
	path string
	url  string
}

func (r fakeRepo) Path() string                                      { return r.path }
func (r fakeRepo) RemoteURL(context.Context, string) (string, error) { return r.url, nil }

type fakeAPI struct {
	mu    sync.Mutex
	calls []string
	auth  []string
	pages map[int][]map[string]any
}

func (a *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	a.calls = append(a.calls, r.URL.Path+"?"+r.URL.RawQuery)
	a.auth = append(a.auth, r.Header.Get("Authorization"))
	a.mu.Unlock()
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	body := a.pages[page]
	if body == nil {
		body = []map[string]any{}
	}
	_ = json.NewEncoder(w).Encode(body)
}

func pr(label, sha, state string, merged bool) map[string]any {
	return map[string]any{"state": state, "merged": merged, "head": map[string]any{"label": label, "ref": label, "sha": sha}}
}

func fill(n int, offset int) []map[string]any {
	out := make([]map[string]any, 0, n)
	for i := range n {
		out = append(out, pr(fmt.Sprintf("p%d", offset+i), "x", "open", false))
	}
	return out
}

func serve(t *testing.T, pages map[int][]map[string]any) (*httptest.Server, *fakeAPI) {
	t.Helper()
	api := &fakeAPI{pages: pages}
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)
	return srv, api
}

func envForge(srv *httptest.Server) *Forge {
	env := map[string]string{envServerURL: srv.URL, envToken: "test-token"}
	return &Forge{Getenv: func(k string) string { return env[k] }, ConfigPaths: []string{}}
}

func TestParseRemote(t *testing.T) {
	cases := map[string]remote{
		"git@git.example.io:owner/repo.git":            {"git.example.io", "owner", "repo"},
		"gitea@Git.Example.io:owner/repo":              {"git.example.io", "owner", "repo"},
		"ssh://git@git.example.io:2222/owner/repo.git": {"git.example.io", "owner", "repo"},
		"https://git.example.io/owner/repo.git":        {"git.example.io", "owner", "repo"},
		"https://git.example.io:3000/sub/owner/repo\n": {"git.example.io", "owner", "repo"},
		"http://user:pw@git.example.io/owner/repo/":    {"git.example.io", "owner", "repo"},
	}
	for in, want := range cases {
		got, err := parseRemote(in)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if got != want {
			t.Errorf("%q: got %v want %v", in, got, want)
		}
	}
	for _, in := range []string{"", "/local/path", "https://git.example.io/repo", "git@host:repo"} {
		if _, err := parseRemote(in); err == nil {
			t.Errorf("%q: expected error", in)
		}
	}
}

func TestMapPulls(t *testing.T) {
	closed, open := gitea.StateClosed, gitea.StateOpen
	prs := []*gitea.PullRequest{
		{State: closed, HasMerged: true, Head: &gitea.PRBranchInfo{Name: "a", Sha: "1"}},
		{State: closed, Head: &gitea.PRBranchInfo{Name: "b", Sha: "2"}},
		{State: open, Head: &gitea.PRBranchInfo{Ref: "c", Sha: "3"}},
		{State: open},
		nil,
	}
	got := mapPulls(prs)
	want := []pull{{"a", "1", tmt.PRMerged}, {"b", "2", tmt.PRClosed}, {"c", "3", tmt.PROpen}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestPickStates(t *testing.T) {
	pulls := []pull{{"a", "1", tmt.PROpen}, {"a", "0", tmt.PRMerged}, {"b", "2", tmt.PRClosed}, {"z", "9", tmt.PROpen}}
	got := pickStates(pulls, []string{"a", "b", "c"})
	want := map[string]tmt.PRState{"a": tmt.PROpen, "b": tmt.PRClosed, "c": tmt.PRNone}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestMergedPulls(t *testing.T) {
	pulls := []pull{{"a", "1", tmt.PROpen}, {"b", "2", tmt.PRMerged}}
	got := mergedPulls(pulls)
	want := []tmt.MergedPR{{Branch: "b", HeadSHA: "2"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestListingPaginatesAndMemoizes(t *testing.T) {
	first := append([]map[string]any{pr("a", "1", "open", false)}, fill(49, 0)...)
	srv, api := serve(t, map[int][]map[string]any{
		1: first,
		2: {pr("b", "2", "closed", true), pr("c", "3", "closed", false)},
		3: fill(50, 100),
	})
	f := envForge(srv)
	repo := fakeRepo{path: "/repo", url: srv.URL + "/o/r.git"}
	ctx := context.Background()

	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			if _, err := f.PRStates(ctx, repo, []string{"a"}); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()

	states, err := f.PRStates(ctx, repo, []string{"a", "b", "c", "d"})
	if err != nil {
		t.Fatal(err)
	}
	if want := (map[string]tmt.PRState{"a": tmt.PROpen, "b": tmt.PRMerged, "c": tmt.PRClosed, "d": tmt.PRNone}); !reflect.DeepEqual(states, want) {
		t.Fatalf("got %v want %v", states, want)
	}
	merged, err := f.MergedBranches(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if want := []tmt.MergedPR{{Branch: "b", HeadSHA: "2"}}; !reflect.DeepEqual(merged, want) {
		t.Fatalf("got %v want %v", merged, want)
	}

	want := []string{
		"/api/v1/repos/o/r/pulls?limit=50&page=1&state=all",
		"/api/v1/repos/o/r/pulls?limit=50&page=2&state=all",
	}
	if !reflect.DeepEqual(api.calls, want) {
		t.Fatalf("calls %q want %q", api.calls, want)
	}
	if api.auth[0] != "token test-token" {
		t.Fatalf("auth header %q", api.auth[0])
	}
}

func TestListingStopsAtMaxPages(t *testing.T) {
	pages := map[int][]map[string]any{}
	for p := 1; p <= 6; p++ {
		pages[p] = fill(50, p*100)
	}
	srv, api := serve(t, pages)
	f := envForge(srv)
	got, err := f.PRStates(context.Background(), fakeRepo{path: "/r", url: srv.URL + "/o/r"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v", got)
	}
	if len(api.calls) != maxPages {
		t.Fatalf("calls %d want %d", len(api.calls), maxPages)
	}
}

func TestListingStopsOnEmptyPage(t *testing.T) {
	srv, api := serve(t, map[int][]map[string]any{1: fill(50, 0)})
	f := envForge(srv)
	if _, err := f.MergedBranches(context.Background(), fakeRepo{path: "/r", url: srv.URL + "/o/r"}); err != nil {
		t.Fatal(err)
	}
	if len(api.calls) != 2 {
		t.Fatalf("calls %q", api.calls)
	}
}

func TestListingError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"message":"nope"}`, http.StatusInternalServerError)
	}))
	defer srv.Close()
	f := envForge(srv)
	repo := fakeRepo{path: "/r", url: srv.URL + "/o/r"}
	if _, err := f.MergedBranches(context.Background(), repo); err == nil {
		t.Fatal("expected error")
	}
	if _, err := f.PRStates(context.Background(), repo, []string{"a"}); err == nil {
		t.Fatal("expected memoized error")
	}
}

const teaYAML = `logins:
  - name: other
    url: https://other.example.io
    token: other-token
  - name: byssh
    url: https://web.example.io
    ssh_host: ssh.example.io:2222
    token: ssh-token
  - name: target
    url: %s
    token: tea-token
    default: true
preferences:
  editor: false
`

func writeConfig(t *testing.T, dir, serverURL string) string {
	t.Helper()
	path := filepath.Join(dir, "tea", "config.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, fmt.Appendf(nil, teaYAML, serverURL), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTeaConfigLogin(t *testing.T) {
	srv, api := serve(t, map[int][]map[string]any{1: {pr("a", "1", "open", false)}})
	xdg := t.TempDir()
	writeConfig(t, xdg, srv.URL)
	env := map[string]string{"XDG_CONFIG_HOME": xdg, "HOME": t.TempDir()}
	f := &Forge{Getenv: func(k string) string { return env[k] }}
	got, err := f.PRStates(context.Background(), fakeRepo{path: "/r", url: srv.URL + "/o/r.git"}, []string{"a"})
	if err != nil {
		t.Fatal(err)
	}
	if got["a"] != tmt.PROpen {
		t.Fatalf("got %v", got)
	}
	if api.auth[0] != "token tea-token" {
		t.Fatalf("auth header %q", api.auth[0])
	}
}

func TestLoginSelection(t *testing.T) {
	home := t.TempDir()
	writeConfig(t, filepath.Join(home, "Library", "Application Support"), "https://git.example.io:3000")
	env := map[string]string{"HOME": home}
	f := &Forge{Getenv: func(k string) string { return env[k] }}
	cases := map[string]string{
		"git.example.io":   "target",
		"other.example.io": "other",
		"web.example.io":   "byssh",
		"ssh.example.io":   "byssh",
	}
	for host, want := range cases {
		l, err := f.login(host)
		if err != nil {
			t.Fatalf("%s: %v", host, err)
		}
		if l.Name != want {
			t.Errorf("%s: got %q want %q", host, l.Name, want)
		}
	}
	if _, err := f.login("missing.example.io"); err == nil {
		t.Fatal("expected error for unknown host")
	}
}

func TestLoginEnvOverride(t *testing.T) {
	cfgDir := t.TempDir()
	writeConfig(t, cfgDir, "https://git.example.io")
	env := map[string]string{"XDG_CONFIG_HOME": cfgDir, envToken: "env-token"}
	f := &Forge{Getenv: func(k string) string { return env[k] }}

	l, err := f.login("git.example.io")
	if err != nil {
		t.Fatal(err)
	}
	if l.URL != "https://git.example.io" || l.Token != "env-token" {
		t.Fatalf("got %+v", l)
	}

	env[envServerURL] = "https://elsewhere.example.io"
	l, err = f.login("git.example.io")
	if err != nil {
		t.Fatal(err)
	}
	if l.Name != "target" {
		t.Fatalf("expected tea login when env server mismatches, got %+v", l.Name)
	}

	env[envServerURL] = "http://git.example.io:3000/"
	l, err = f.login("git.example.io")
	if err != nil {
		t.Fatal(err)
	}
	if l.URL != "http://git.example.io:3000/" || l.Token != "env-token" {
		t.Fatalf("got %+v", l)
	}
}

func TestLoginNoConfig(t *testing.T) {
	f := &Forge{Getenv: func(string) string { return "" }, ConfigPaths: []string{filepath.Join(t.TempDir(), "missing.yml")}}
	if _, err := f.login("git.example.io"); err == nil {
		t.Fatal("expected error")
	}
}

func TestTeaConfigPaths(t *testing.T) {
	env := map[string]string{"XDG_CONFIG_HOME": "/xdg", "HOME": "/home/u"}
	got := teaConfigPaths(func(k string) string { return env[k] })
	want := []string{
		"/xdg/tea/config.yml",
		"/home/u/.config/tea/config.yml",
		"/home/u/Library/Application Support/tea/config.yml",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}
