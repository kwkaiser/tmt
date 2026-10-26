package main

import (
	"os"

	"git.kwkaiser.io/kwkaiser/tmt/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
