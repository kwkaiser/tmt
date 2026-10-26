package gitea

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

type fakeRepo struct {
	tmt.Repo
	path string
}

func (r fakeRepo) Path() string { return r.path }

func TestParsePage(t *testing.T) {
	data := []byte(`[
		{"state":"closed","merged":true,"head":{"label":"a","sha":"1"}},
		{"state":"closed","merged":false,"head":{"label":"b","sha":"2"}},
		{"state":"open","merged":false,"head":{"label":"c","sha":"3"}}
	]`)
	got, ok := parsePage(data)
	if !ok {
		t.Fatal("expected ok")
	}
	want := []pull{{"a", "1", tmt.PRMerged}, {"b", "2", tmt.PRClosed}, {"c", "3", tmt.PROpen}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestParsePageStops(t *testing.T) {
	for _, in := range []string{"[]", `{"message":"x"}`, "", "garbage"} {
		if _, ok := parsePage([]byte(in)); ok {
			t.Fatalf("expected stop for %q", in)
		}
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

func fakeTea(t *testing.T, log string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "tea")
	script := `#!/bin/sh
echo "$2" >> "` + log + `"
case "$2" in
*page=1) echo '[{"state":"open","merged":false,"head":{"label":"a","sha":"1"}}]' ;;
*page=2) echo '[{"state":"closed","merged":true,"head":{"label":"b","sha":"2"}}]' ;;
*) echo '[]' ;;
esac
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

func TestListingPaginatesAndMemoizes(t *testing.T) {
	log := filepath.Join(t.TempDir(), "calls")
	f := &Forge{Bin: fakeTea(t, log)}
	repo := fakeRepo{path: t.TempDir()}
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

	states, err := f.PRStates(ctx, repo, []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if want := (map[string]tmt.PRState{"a": tmt.PROpen, "b": tmt.PRMerged}); !reflect.DeepEqual(states, want) {
		t.Fatalf("got %v want %v", states, want)
	}
	merged, err := f.MergedBranches(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if want := []tmt.MergedPR{{Branch: "b", HeadSHA: "2"}}; !reflect.DeepEqual(merged, want) {
		t.Fatalf("got %v want %v", merged, want)
	}

	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{pageEndpoint(1), pageEndpoint(2), pageEndpoint(3)}
	if got := strings.Fields(string(calls)); !reflect.DeepEqual(got, want) {
		t.Fatalf("calls %q want %q", got, want)
	}
}

func TestListingError(t *testing.T) {
	f := &Forge{Bin: filepath.Join(t.TempDir(), "missing")}
	if _, err := f.MergedBranches(context.Background(), fakeRepo{path: t.TempDir()}); err == nil {
		t.Fatal("expected error")
	}
}
