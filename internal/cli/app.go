package cli

import (
	"os"
	"path/filepath"

	"git.kwkaiser.io/kwkaiser/tmt/internal/claude"
	"git.kwkaiser.io/kwkaiser/tmt/internal/clone"
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
	cfg, bc, err := loadConfig(os.Getenv, os.Executable)
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
		Agent:  claude.New(bc.ClaudeCommand),
		Env:    direnv.New(),
		Out:    os.Stdout,
		Err:    os.Stderr,
	}, p.Close, nil
}
