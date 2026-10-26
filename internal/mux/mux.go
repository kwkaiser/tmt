package mux

import (
	"fmt"
	"os"

	"git.kwkaiser.io/kwkaiser/tmt/internal/herdr"
	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
	"git.kwkaiser.io/kwkaiser/tmt/internal/tmux"
)

const (
	Tmux  = "tmux"
	Herdr = "herdr"
)

func FromEnv() (tmt.Mux, error) {
	return New(os.Getenv)
}

func New(getenv func(string) string) (tmt.Mux, error) {
	switch kind := Kind(getenv); kind {
	case Tmux:
		return &tmux.Tmux{Bin: "tmux", Getenv: getenv}, nil
	case Herdr:
		bin := getenv("HERDR_BIN_PATH")
		if bin == "" {
			bin = "herdr"
		}
		return &herdr.Herdr{Bin: bin, SortBin: "herdr-sort-tabs", Getenv: getenv}, nil
	default:
		return nil, fmt.Errorf("unknown mux %q", kind)
	}
}

func Kind(getenv func(string) string) string {
	if m := getenv("TMT_MUX"); m != "" {
		return m
	}
	if getenv("HERDR_ENV") == "1" {
		return Herdr
	}
	return Tmux
}
