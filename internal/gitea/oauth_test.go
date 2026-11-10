package gitea

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
	"github.com/go-signet/sdk-go/credstore"
)

type fakeCreds struct {
	mu        sync.Mutex
	tok       credstore.Token
	err       error
	next      credstore.Token
	refreshes int
	dirs      []string
}

func (c *fakeCreds) loader(dir string) tokenLoader {
	c.mu.Lock()
	c.dirs = append(c.dirs, dir)
	c.mu.Unlock()
	return func(string) (credstore.Token, error) {
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.tok, c.err
	}
}

func (c *fakeCreds) refresh(context.Context, string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.refreshes++
	c.tok, c.err = c.next, nil
	return nil
}

func TestOAuthToken(t *testing.T) {
	now := time.Now()
	cases := map[string]struct {
		tok       credstore.Token
		err       error
		want      string
		refreshes int
	}{
		"fresh":     {tok: credstore.Token{AccessToken: "a", ExpiresAt: now.Add(time.Hour)}, want: "a"},
		"no expiry": {tok: credstore.Token{AccessToken: "a"}, want: "a"},
		"expiring":  {tok: credstore.Token{AccessToken: "a", ExpiresAt: now.Add(time.Minute)}, want: "b", refreshes: 1},
		"expired":   {tok: credstore.Token{AccessToken: "a", ExpiresAt: now.Add(-time.Hour)}, want: "b", refreshes: 1},
		"missing":   {err: credstore.ErrNotFound, want: "b", refreshes: 1},
		"empty":     {tok: credstore.Token{}, want: "b", refreshes: 1},
	}
	for name, tc := range cases {
		c := &fakeCreds{tok: tc.tok, err: tc.err, next: credstore.Token{AccessToken: "b", ExpiresAt: now.Add(time.Hour)}}
		got, err := oauthToken(context.Background(), "l", c.loader(""), c.refresh)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got != tc.want || c.refreshes != tc.refreshes {
			t.Errorf("%s: got %q after %d refreshes, want %q after %d", name, got, c.refreshes, tc.want, tc.refreshes)
		}
	}
}

func TestOAuthTokenRefreshError(t *testing.T) {
	load := func(string) (credstore.Token, error) { return credstore.Token{}, credstore.ErrNotFound }
	boom := errors.New("boom")
	_, err := oauthToken(context.Background(), "l", load, func(context.Context, string) error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
}

func TestOAuthTokenStillEmptyAfterRefresh(t *testing.T) {
	c := &fakeCreds{err: credstore.ErrNotFound}
	if _, err := oauthToken(context.Background(), "l", c.loader(""), c.refresh); err == nil {
		t.Fatal("expected error")
	}
}

const oauthYAML = `logins:
  - name: target
    url: %s
    auth_method: oauth
`

func TestTeaConfigOAuthLogin(t *testing.T) {
	srv, api := serve(t, map[int][]map[string]any{1: {pr("a", "1", "open", false)}})
	xdg := t.TempDir()
	path := filepath.Join(xdg, "tea", "config.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, fmt.Appendf(nil, oauthYAML, srv.URL), 0o600); err != nil {
		t.Fatal(err)
	}
	c := &fakeCreds{err: credstore.ErrNotFound, next: credstore.Token{AccessToken: "oauth-token"}}
	env := map[string]string{"XDG_CONFIG_HOME": xdg}
	f := &Forge{Getenv: func(k string) string { return env[k] }, loadToken: c.loader, refresh: c.refresh}

	for _, p := range []string{"/r1", "/r2"} {
		got, err := f.PRStates(context.Background(), fakeRepo{path: p, url: srv.URL + "/o/r.git"}, []string{"a"})
		if err != nil {
			t.Fatal(err)
		}
		if got["a"] != tmt.PROpen {
			t.Fatalf("got %v", got)
		}
	}
	for _, h := range api.auth {
		if h != "token oauth-token" {
			t.Fatalf("auth header %q", h)
		}
	}
	if c.refreshes != 1 || len(c.dirs) != 1 || c.dirs[0] != filepath.Dir(path) {
		t.Fatalf("refreshes=%d dirs=%q", c.refreshes, c.dirs)
	}
}
