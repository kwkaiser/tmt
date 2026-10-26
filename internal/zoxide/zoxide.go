package zoxide

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

type Index struct {
	Bin string
}

var _ tmt.DirIndex = (*Index)(nil)

func New() *Index {
	return &Index{Bin: "zoxide"}
}

func (i *Index) bin() string {
	if i.Bin == "" {
		return "zoxide"
	}
	return i.Bin
}

func (i *Index) run(ctx context.Context, args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, i.bin(), args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("zoxide %s: %w: %s", strings.Join(args, " "), err, msg)
		}
		return "", fmt.Errorf("zoxide %s: %w", strings.Join(args, " "), err)
	}
	return stdout.String(), nil
}

func (i *Index) Query(ctx context.Context, q string) (string, error) {
	out, err := i.run(ctx, "query", "--", q)
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && ctx.Err() == nil {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (i *Index) List(ctx context.Context) ([]string, error) {
	out, err := i.run(ctx, "query", "-l")
	if err != nil {
		return nil, err
	}
	var dirs []string
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			dirs = append(dirs, line)
		}
	}
	return dirs, nil
}
