package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

const defaultBin = "gh"

type Forge struct {
	Bin string
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
	if len(branches) == 0 {
		return map[string]tmt.PRState{}, nil
	}
	out, err := f.run(ctx, repo.Path(), graphqlArgs(branches)...)
	if err != nil {
		return nil, err
	}
	return parseStates(out, branches)
}

func (f *Forge) MergedBranches(ctx context.Context, repo tmt.Repo) ([]tmt.MergedPR, error) {
	out, err := f.run(ctx, repo.Path(), "pr", "list", "--state", "merged", "--author", "@me", "--limit", "200", "--json", "headRefName,headRefOid")
	if err != nil {
		return nil, err
	}
	return parseMerged(out)
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

func graphqlQuery(n int) string {
	var decl, body strings.Builder
	for i := range n {
		fmt.Fprintf(&decl, ",$h%d:String!", i)
		fmt.Fprintf(&body, "b%d:pullRequests(headRefName:$h%d,first:1,orderBy:{field:CREATED_AT,direction:DESC}){nodes{state}} ", i, i)
	}
	return "query($owner:String!,$repo:String!" + decl.String() + "){repository(owner:$owner,name:$repo){" + body.String() + "}}"
}

func graphqlArgs(branches []string) []string {
	args := []string{"api", "graphql", "-F", "owner={owner}", "-F", "repo={repo}"}
	for i, b := range branches {
		args = append(args, "-f", "h"+strconv.Itoa(i)+"="+b)
	}
	return append(args, "-f", "query="+graphqlQuery(len(branches)))
}

type stateNodes struct {
	Nodes []struct {
		State string `json:"state"`
	} `json:"nodes"`
}

func parseStates(data []byte, branches []string) (map[string]tmt.PRState, error) {
	var resp struct {
		Data struct {
			Repository map[string]stateNodes `json:"repository"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse graphql response: %w", err)
	}
	states := map[string]tmt.PRState{}
	for key, v := range resp.Data.Repository {
		i, err := strconv.Atoi(strings.TrimPrefix(key, "b"))
		if err != nil || i < 0 || i >= len(branches) {
			continue
		}
		states[branches[i]] = nodeState(v)
	}
	return states, nil
}

func nodeState(v stateNodes) tmt.PRState {
	if len(v.Nodes) == 0 || v.Nodes[0].State == "" {
		return tmt.PRNone
	}
	return tmt.PRState(v.Nodes[0].State)
}

func parseMerged(data []byte) ([]tmt.MergedPR, error) {
	var prs []struct {
		HeadRefName string `json:"headRefName"`
		HeadRefOid  string `json:"headRefOid"`
	}
	if err := json.Unmarshal(data, &prs); err != nil {
		return nil, fmt.Errorf("parse pr list: %w", err)
	}
	merged := make([]tmt.MergedPR, 0, len(prs))
	for _, p := range prs {
		merged = append(merged, tmt.MergedPR{Branch: p.HeadRefName, HeadSHA: p.HeadRefOid})
	}
	return merged, nil
}
