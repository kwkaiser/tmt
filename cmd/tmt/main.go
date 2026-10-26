package main

import (
	"os"

	"git.kwkaiser.io/kwkaiser/tmt/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
