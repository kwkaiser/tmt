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
	info, err := os.Stat(filepath.Join(dir, ".envrc"))
	if err != nil || !info.Mode().IsRegular() {
		return nil
	}
	bin, err := exec.LookPath(d.bin())
	if err != nil {
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
