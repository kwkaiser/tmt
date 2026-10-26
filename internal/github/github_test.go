package github

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

type fakeRepo struct {
	tmt.Repo
	path string
}

func (r fakeRepo) Path() string { return r.path }

func TestGraphqlQuery(t *testing.T) {
	got := graphqlQuery(2)
	want := "query($owner:String!,$repo:String!,$h0:String!,$h1:String!){repository(owner:$owner,name:$repo){" +
		"b0:pullRequests(headRefName:$h0,first:1,orderBy:{field:CREATED_AT,direction:DESC}){nodes{state}} " +
		"b1:pullRequests(headRefName:$h1,first:1,orderBy:{field:CREATED_AT,direction:DESC}){nodes{state}} }}"
	if got != want {
		t.Fatalf("query mismatch\ngot:  %s\nwant: %s", got, want)
	}
}

func TestGraphqlArgs(t *testing.T) {
	got := graphqlArgs([]string{"a/x", "b"})
	want := []string{"api", "graphql", "-F", "owner={owner}", "-F", "repo={repo}", "-f", "h0=a/x", "-f", "h1=b", "-f", "query=" + graphqlQuery(2)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestParseStates(t *testing.T) {
	data := []byte(`{"data":{"repository":{
		"b0":{"nodes":[{"state":"MERGED"}]},
		"b1":{"nodes":[]},
		"b2":{"nodes":[{"state":"OPEN"}]},
		"b9":{"nodes":[{"state":"OPEN"}]}
	}}}`)
	got, err := parseStates(data, []string{"one", "two", "three"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]tmt.PRState{"one": tmt.PRMerged, "two": tmt.PRNone, "three": tmt.PROpen}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestParseStatesInvalid(t *testing.T) {
	if _, err := parseStates([]byte("nope"), []string{"a"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseMerged(t *testing.T) {
	got, err := parseMerged([]byte(`[{"headRefName":"feat","headRefOid":"abc"},{"headRefName":"fix","headRefOid":"def"}]`))
	if err != nil {
		t.Fatal(err)
	}
	want := []tmt.MergedPR{{Branch: "feat", HeadSHA: "abc"}, {Branch: "fix", HeadSHA: "def"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestPRStatesEmptySkipsBinary(t *testing.T) {
	f := &Forge{Bin: filepath.Join(t.TempDir(), "missing")}
	got, err := f.PRStates(context.Background(), fakeRepo{path: t.TempDir()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestPRStatesRunsInRepoDir(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(t.TempDir(), "gh")
	script := "#!/bin/sh\npwd -P > \"$TMT_TEST_OUT\"\necho '{\"data\":{\"repository\":{\"b0\":{\"nodes\":[{\"state\":\"CLOSED\"}]}}}}'\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "pwd")
	t.Setenv("TMT_TEST_OUT", out)
	f := &Forge{Bin: bin}
	got, err := f.PRStates(context.Background(), fakeRepo{path: dir}, []string{"b"})
	if err != nil {
		t.Fatal(err)
	}
	if got["b"] != tmt.PRClosed {
		t.Fatalf("got %v", got)
	}
	pwd, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	real, _ := filepath.EvalSymlinks(dir)
	if strings.TrimSpace(string(pwd)) != real {
		t.Fatalf("ran in %q want %q", pwd, real)
	}
}
