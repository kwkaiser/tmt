package cli

import (
	"strings"

	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

type backendConfig struct {
	FZFColors     string
	Popup         bool
	ClaudeCommand string
}

func envOr(getenv func(string) string, name, fallback string) string {
	if v := getenv(name); v != "" {
		return v
	}
	return fallback
}

func loadConfig(getenv func(string) string, executable func() (string, error)) (tmt.Config, backendConfig, error) {
	self, err := executable()
	if err != nil {
		return tmt.Config{}, backendConfig{}, err
	}
	cfg := tmt.Config{
		Root:         envOr(getenv, "TMT_ROOT", getenv("HOME")+"/tasks"),
		BranchPrefix: envOr(getenv, "TMT_BRANCH_PREFIX", "u/kwkaiser/"),
		CloneNames:   strings.Fields(envOr(getenv, "TMT_CLONE", "node_modules .env .env.local .env.mcp")),
		Self:         self,
		Theme: tmt.Theme{
			Worktree: envOr(getenv, "TMT_COLOR_WORKTREE", "38;5;114"),
			Main:     envOr(getenv, "TMT_COLOR_MAIN", "38;5;244"),
			Root:     envOr(getenv, "TMT_COLOR_ROOT", "38;5;75"),
			Accent:   envOr(getenv, "TMT_COLOR_ACCENT", "38;5;221"),
			Dim:      envOr(getenv, "TMT_COLOR_DIM", "38;5;246"),
		},
	}
	backends := backendConfig{
		FZFColors:     getenv("TMT_FZF_COLORS"),
		Popup:         getenv("TMT_POPUP") != "",
		ClaudeCommand: envOr(getenv, "TMT_CLAUDE_COMMAND", "ccp"),
	}
	return cfg, backends, nil
}
