package agents

import (
	"cmp"
	"path/filepath"
	"slices"
	"strings"

	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

type Rule struct {
	Dir   string
	Agent tmt.Agent
}

type Selector struct {
	Default tmt.Agent
	rules   []Rule
}

var _ tmt.AgentSelector = (*Selector)(nil)

func New(def tmt.Agent, rules []Rule) *Selector {
	sorted := make([]Rule, len(rules))
	for i, r := range rules {
		sorted[i] = Rule{Dir: filepath.Clean(r.Dir), Agent: r.Agent}
	}
	slices.SortStableFunc(sorted, func(a, b Rule) int {
		return cmp.Compare(len(b.Dir), len(a.Dir))
	})
	return &Selector{Default: def, rules: sorted}
}

func (s *Selector) For(base string) tmt.Agent {
	base = filepath.Clean(base)
	for _, r := range s.rules {
		if base == r.Dir || strings.HasPrefix(base, strings.TrimSuffix(r.Dir, string(filepath.Separator))+string(filepath.Separator)) {
			return r.Agent
		}
	}
	return s.Default
}
