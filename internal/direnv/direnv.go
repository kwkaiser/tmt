package direnv

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

type Direnv struct {
	Bin string
}

var _ tmt.EnvTrust = (*Direnv)(nil)

func New() *Direnv {
	return &Direnv{Bin: "direnv"}
}

func (d *Direnv) bin() string {
	if d.Bin == "" {
		return "direnv"
	}
	return d.Bin
}

func (d *Direnv) Allow(ctx context.Context, dir string) error {
	if !isRegular(filepath.Join(dir, ".envrc")) {
		return nil
	}
	bin, ok := lookPath(d.bin())
	if !ok {
		return nil
	}
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, bin, "allow", dir)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("direnv allow %s: %w: %s", dir, err, msg)
		}
		return fmt.Errorf("direnv allow %s: %w", dir, err)
	}
	return nil
}

func isRegular(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func lookPath(name string) (string, bool) {
	p, err := exec.LookPath(name)
	return p, err == nil
}
