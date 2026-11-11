package agents

import (
	"context"
	"testing"

	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

type fake struct{ name string }

func (f *fake) Command(string) string                                  { return f.name }
func (f *fake) Trust(context.Context, string) error                    { return nil }
func (f *fake) WriteContext(context.Context, tmt.Task, []string) error { return nil }
func (f *fake) ReadContext(context.Context, tmt.Task) (string, error)  { return "", nil }

func TestFor(t *testing.T) {
	def, a, b := &fake{"def"}, &fake{"a"}, &fake{"b"}
	s := New(def, []Rule{
		{Dir: "/home/k/work", Agent: a},
		{Dir: "/home/k/work/special/", Agent: b},
	})
	for base, want := range map[string]*fake{
		"/home/k/work":             a,
		"/home/k/work/api":         a,
		"/home/k/work/special":     b,
		"/home/k/work/special/x":   b,
		"/home/k/workshop":         def,
		"/home/k/work/specialists": a,
		"/srv":                     def,
	} {
		if got := s.For(base); got != tmt.Agent(want) {
			t.Errorf("For(%q) = %v; want %v", base, got, want.name)
		}
	}
}

func TestForRoot(t *testing.T) {
	def, root := &fake{"def"}, &fake{"root"}
	s := New(def, []Rule{{Dir: "/", Agent: root}})
	if got := s.For("/srv/x"); got != tmt.Agent(root) {
		t.Fatalf("For = %v; want root", got)
	}
}
