package tmt_test

import (
	"reflect"
	"strings"
	"testing"

	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

func TestWindowCreatesAndRuns(t *testing.T) {
	e := newEnv(t)
	e.mux.current = "w1"
	err := e.app.Window(t.Context(), tmt.WindowOptions{Name: "api", Dir: "/t/api", Command: []string{"nvim", "+3", "/t/api/a b.go"}})
	if err != nil {
		t.Fatal(err)
	}
	if want := []tmt.Window{{Name: "api", Dir: "/t/api"}}; !reflect.DeepEqual(e.mux.created, want) {
		t.Fatalf("created = %v; want %v", e.mux.created, want)
	}
	if got, want := e.mux.ran["%new"], `nvim +3 '/t/api/a b.go'`; got != want {
		t.Fatalf("ran = %q; want %q", got, want)
	}
	if want := []string{"%new"}; !reflect.DeepEqual(e.mux.focused, want) {
		t.Fatalf("focused = %v; want %v", e.mux.focused, want)
	}
}

func TestWindowReusesIdlePane(t *testing.T) {
	e := newEnv(t)
	e.mux.windows = map[string]tmt.WindowID{"s/api": "@1"}
	e.mux.panes = map[tmt.WindowID][]tmt.Pane{"@1": {{ID: "%1"}, {ID: "%2", Idle: true}}}
	err := e.app.Window(t.Context(), tmt.WindowOptions{Session: "s", Name: "api", Dir: "/t/it's", Command: []string{"nvim", "x"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(e.mux.created) != 0 || len(e.mux.splits) != 0 {
		t.Fatalf("created = %v, splits = %v; want none", e.mux.created, e.mux.splits)
	}
	if got, want := e.mux.ran["%2"], `cd '/t/it'\''s' && nvim x`; got != want {
		t.Fatalf("ran = %q; want %q", got, want)
	}
}

func TestWindowSplitsBusyWindow(t *testing.T) {
	e := newEnv(t)
	e.mux.windows = map[string]tmt.WindowID{"s/api": "@1"}
	e.mux.panes = map[tmt.WindowID][]tmt.Pane{"@1": {{ID: "%1"}}}
	if err := e.app.Window(t.Context(), tmt.WindowOptions{Session: "s", Name: "api", Dir: "/t/api", Command: []string{"nvim"}}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"@1 /t/api"}; !reflect.DeepEqual(e.mux.splits, want) {
		t.Fatalf("splits = %v; want %v", e.mux.splits, want)
	}
	if got := e.mux.ran["%split"]; got != "nvim" {
		t.Fatalf("ran = %q; want nvim", got)
	}
}

func TestWindowWithoutCommandSelects(t *testing.T) {
	e := newEnv(t)
	e.mux.windows = map[string]tmt.WindowID{"s/api": "@1"}
	if err := e.app.Window(t.Context(), tmt.WindowOptions{Session: "s", Name: "api", Dir: "/t/api"}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"@1"}; !reflect.DeepEqual(e.mux.focused, want) || len(e.mux.ran) != 0 {
		t.Fatalf("focused = %v, ran = %v", e.mux.focused, e.mux.ran)
	}
}

func TestWindowOutsideSession(t *testing.T) {
	e := newEnv(t)
	err := e.app.Window(t.Context(), tmt.WindowOptions{Name: "api", Dir: "/t/api"})
	if err == nil || !strings.Contains(err.Error(), "--session") {
		t.Fatalf("err = %v; want --session hint", err)
	}
}
