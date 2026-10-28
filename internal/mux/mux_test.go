package mux

import (
	"testing"

	"git.kwkaiser.io/kwkaiser/tmt/internal/herdr"
	"git.kwkaiser.io/kwkaiser/tmt/internal/tmux"
)

func env(kv map[string]string) func(string) string {
	return func(k string) string { return kv[k] }
}

func TestKind(t *testing.T) {
	cases := []struct {
		env  map[string]string
		want string
	}{
		{nil, Tmux},
		{map[string]string{"HERDR_ENV": "1"}, Herdr},
		{map[string]string{"HERDR_ENV": "0"}, Tmux},
		{map[string]string{"TMT_MUX": "tmux", "HERDR_ENV": "1"}, Tmux},
		{map[string]string{"TMT_MUX": "herdr"}, Herdr},
		{map[string]string{"TMT_MUX": "screen"}, "screen"},
	}
	for _, c := range cases {
		if got := Kind(env(c.env)); got != c.want {
			t.Errorf("Kind(%v) = %q; want %q", c.env, got, c.want)
		}
	}
}

func TestNewTmux(t *testing.T) {
	m, err := New(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m.(*tmux.Tmux); !ok {
		t.Fatalf("New = %T; want *tmux.Tmux", m)
	}
}

func TestNewHerdr(t *testing.T) {
	m, err := New(env(map[string]string{"HERDR_ENV": "1"}))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m.(*herdr.Herdr); !ok {
		t.Fatalf("New = %T; want *herdr.Herdr", m)
	}
}

func TestNewHerdrExplicit(t *testing.T) {
	m, err := New(env(map[string]string{"TMT_MUX": "herdr"}))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m.(*herdr.Herdr); !ok {
		t.Fatalf("New = %T; want *herdr.Herdr", m)
	}
}

func TestNewUnknown(t *testing.T) {
	if _, err := New(env(map[string]string{"TMT_MUX": "screen"})); err == nil {
		t.Fatal("expected error for unknown mux")
	}
}
