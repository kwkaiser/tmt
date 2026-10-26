package tmux

import (
	"testing"

	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

func TestFindWindow(t *testing.T) {
	out := "@1 claude\n@2 api\n@3 web\n@4 api"
	cases := []struct {
		name string
		id   tmt.WindowID
		ok   bool
	}{
		{"claude", "@1", true},
		{"api", "@2", true},
		{"web", "@3", true},
		{"missing", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		id, ok := findWindow(out, c.name)
		if id != c.id || ok != c.ok {
			t.Errorf("findWindow(%q) = %q, %v; want %q, %v", c.name, id, ok, c.id, c.ok)
		}
	}
}

func TestFindWindowEmpty(t *testing.T) {
	if _, ok := findWindow("", "claude"); ok {
		t.Fatal("expected no match on empty output")
	}
}

func TestCurrentSessionOutsideTmux(t *testing.T) {
	m := &Tmux{Bin: "/nonexistent/tmux", Getenv: func(string) string { return "" }}
	s, err := m.CurrentSession(t.Context())
	if err != nil || s != "" {
		t.Fatalf("CurrentSession = %q, %v; want empty", s, err)
	}
}
