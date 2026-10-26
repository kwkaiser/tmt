package cli

import (
	"os"

	"git.kwkaiser.io/kwkaiser/tmt/internal/clone"
	"git.kwkaiser.io/kwkaiser/tmt/internal/forge"
	"git.kwkaiser.io/kwkaiser/tmt/internal/gitcli"
	"git.kwkaiser.io/kwkaiser/tmt/internal/mux"
	"git.kwkaiser.io/kwkaiser/tmt/internal/prompt"
	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

func newApp() (app, func() error, error) {
	cfg, _, err := loadConfig(os.Getenv, os.Executable)
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
		Git:    gitcli.New(),
		Forges: forge.NewResolver(),
		Mux:    m,
		Prompt: p,
		Cloner: clone.New(),
		Out:    os.Stdout,
		Err:    os.Stderr,
	}, p.Close, nil
}
