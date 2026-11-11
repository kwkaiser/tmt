package cli

import (
	"os"
	"path/filepath"

	"git.kwkaiser.io/kwkaiser/tmt/internal/agents"
	"git.kwkaiser.io/kwkaiser/tmt/internal/claude"
	"git.kwkaiser.io/kwkaiser/tmt/internal/clone"
	"git.kwkaiser.io/kwkaiser/tmt/internal/codex"
	"git.kwkaiser.io/kwkaiser/tmt/internal/devshell"
	"git.kwkaiser.io/kwkaiser/tmt/internal/direnv"
	"git.kwkaiser.io/kwkaiser/tmt/internal/forge"
	"git.kwkaiser.io/kwkaiser/tmt/internal/fzf"
	"git.kwkaiser.io/kwkaiser/tmt/internal/gitcli"
	"git.kwkaiser.io/kwkaiser/tmt/internal/mux"
	"git.kwkaiser.io/kwkaiser/tmt/internal/prompt"
	"git.kwkaiser.io/kwkaiser/tmt/internal/taskstore"
	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
	"git.kwkaiser.io/kwkaiser/tmt/internal/trash"
	"git.kwkaiser.io/kwkaiser/tmt/internal/zoxide"
)

func newApp() (app, func() error, error) {
	cfg, bc, err := loadConfig(os.Getenv, os.Executable, os.ReadFile)
	if err != nil {
		return nil, nil, err
	}
	m, err := mux.New(os.Getenv)
	if err != nil {
		return nil, nil, err
	}
	p := prompt.New()
	return &tmt.App{
		Config: cfg,
		Tasks:  taskstore.New(cfg.Root),
		Git:    gitcli.New(),
		Forges: forge.NewResolver(),
		Mux:    m,
		Picker: fzf.New(bc.FZFColors, bc.Popup),
		Prompt: p,
		Cloner: clone.New(),
		Trash:  trash.New(filepath.Join(cfg.Root, ".trash")),
		Dirs:   zoxide.New(),
		Agents: newAgents(bc),
		Env:    direnv.New(),
		Shell:  devshell.New(),
		Out:    os.Stdout,
		Err:    os.Stderr,
	}, p.Close, nil
}

func newAgents(bc backendConfig) *agents.Selector {
	byName := map[string]tmt.Agent{
		"claude": claude.New(bc.AgentCommands["claude"]),
		"codex":  codex.New(bc.AgentCommands["codex"]),
	}
	var rules []agents.Rule
	for dir, name := range bc.AgentDirs {
		rules = append(rules, agents.Rule{Dir: dir, Agent: byName[name]})
	}
	return agents.New(byName[bc.Agent], rules)
}
