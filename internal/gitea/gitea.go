package gitea

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

const (
	defaultBin = "tea"
	maxPages   = 4
)

type pull struct {
	Branch string
	SHA    string
	State  tmt.PRState
}

type listing struct {
	once  sync.Once
	pulls []pull
	err   error
}

type Forge struct {
	Bin string

	mu    sync.Mutex
	cache map[string]*listing
}

func New() *Forge {
	return &Forge{Bin: defaultBin}
}

func (f *Forge) bin() string {
	if f.Bin == "" {
		return defaultBin
	}
	return f.Bin
}

func (f *Forge) PRStates(ctx context.Context, repo tmt.Repo, branches []string) (map[string]tmt.PRState, error) {
	pulls, err := f.pulls(ctx, repo.Path())
	if err != nil {
		return nil, err
	}
	return pickStates(pulls, branches), nil
}

func (f *Forge) MergedBranches(ctx context.Context, repo tmt.Repo) ([]tmt.MergedPR, error) {
	pulls, err := f.pulls(ctx, repo.Path())
	if err != nil {
		return nil, err
	}
	return mergedPulls(pulls), nil
}

func (f *Forge) pulls(ctx context.Context, dir string) ([]pull, error) {
	f.mu.Lock()
	if f.cache == nil {
		f.cache = map[string]*listing{}
	}
	l, ok := f.cache[dir]
	if !ok {
		l = &listing{}
		f.cache[dir] = l
	}
	f.mu.Unlock()
	l.once.Do(func() {
		l.pulls, l.err = f.list(ctx, dir)
	})
	return l.pulls, l.err
}

func (f *Forge) list(ctx context.Context, dir string) ([]pull, error) {
	var all []pull
	for page := 1; page <= maxPages; page++ {
		out, err := f.run(ctx, dir, "api", pageEndpoint(page))
		if err != nil {
			return nil, err
		}
		pulls, ok := parsePage(out)
		if !ok {
			break
		}
		all = append(all, pulls...)
	}
	return all, nil
}

func (f *Forge) run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, f.bin(), args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("%s %s: %w: %s", f.bin(), args[0], err, msg)
		}
		return nil, fmt.Errorf("%s %s: %w", f.bin(), args[0], err)
	}
	return out, nil
}

func pageEndpoint(page int) string {
	return fmt.Sprintf("repos/{owner}/{repo}/pulls?state=all&limit=50&page=%d", page)
}

func parsePage(data []byte) ([]pull, bool) {
	var raw []struct {
		State  string `json:"state"`
		Merged bool   `json:"merged"`
		Head   struct {
			Label string `json:"label"`
			SHA   string `json:"sha"`
		} `json:"head"`
	}
	if err := json.Unmarshal(data, &raw); err != nil || len(raw) == 0 {
		return nil, false
	}
	pulls := make([]pull, 0, len(raw))
	for _, r := range raw {
		pulls = append(pulls, pull{Branch: r.Head.Label, SHA: r.Head.SHA, State: pullState(r.Merged, r.State)})
	}
	return pulls, true
}

func pullState(merged bool, state string) tmt.PRState {
	switch {
	case merged:
		return tmt.PRMerged
	case state == "closed":
		return tmt.PRClosed
	default:
		return tmt.PROpen
	}
}

func pickStates(pulls []pull, branches []string) map[string]tmt.PRState {
	states := make(map[string]tmt.PRState, len(branches))
	for _, b := range branches {
		states[b] = tmt.PRNone
	}
	seen := map[string]bool{}
	for _, p := range pulls {
		if seen[p.Branch] {
			continue
		}
		seen[p.Branch] = true
		if _, ok := states[p.Branch]; ok {
			states[p.Branch] = p.State
		}
	}
	return states
}

func mergedPulls(pulls []pull) []tmt.MergedPR {
	var merged []tmt.MergedPR
	for _, p := range pulls {
		if p.State == tmt.PRMerged {
			merged = append(merged, tmt.MergedPR{Branch: p.Branch, HeadSHA: p.SHA})
		}
	}
	return merged
}
