package fzf

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

type Picker struct {
	Bin    string
	Colors string
	Popup  bool
}

var _ tmt.Picker = (*Picker)(nil)

func New(colors string, popup bool) *Picker {
	return &Picker{Bin: "fzf", Colors: colors, Popup: popup}
}

func (p *Picker) bin() string {
	if p.Bin == "" {
		return "fzf"
	}
	return p.Bin
}

func (p *Picker) Pick(ctx context.Context, pk tmt.Pick) (tmt.Picked, bool, error) {
	var stdout bytes.Buffer
	cmd := exec.CommandContext(ctx, p.bin(), Args(pk, p.Colors, p.Popup)...)
	cmd.Stdin = strings.NewReader(Input(pk.Items))
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return tmt.Picked{}, false, ctx.Err()
		}
		if exit, ok := errors.AsType[*exec.ExitError](err); ok {
			switch exit.ExitCode() {
			case 1, 130:
				return tmt.Picked{}, false, nil
			}
		}
		return tmt.Picked{}, false, fmt.Errorf("fzf: %w", err)
	}
	return Parse(stdout.String(), pk), true, nil
}

func Input(items []tmt.PickItem) string {
	var b strings.Builder
	for _, it := range items {
		b.WriteString(it.Key)
		b.WriteByte('\t')
		b.WriteString(it.Display)
		b.WriteByte('\n')
	}
	return b.String()
}

func Args(pk tmt.Pick, colors string, popup bool) []string {
	args := []string{
		"--ansi", "--layout=reverse", "--info=inline-right", "--highlight-line",
		"--prompt=❯ ", "--pointer=▌", "--gutter= ", "--marker=▍", "--separator=─", "--scrollbar=│",
	}
	if colors != "" {
		args = append(args, "--color="+colors)
	}
	args = append(args, `--delimiter=\t`, "--with-nth=2..")
	if len(pk.Preview) > 0 {
		args = append(args, "--preview="+Preview(pk.Preview))
	}
	args = append(args, "--preview-window=right,55%,border-left,<60(down,50%,border-top)")
	if popup {
		args = append(args, "--border=none", "--input-border=line", "--input-label= "+pk.Label+" ", "--no-separator")
	} else {
		args = append(args, "--border=rounded", "--border-label= "+pk.Label+" ", "--padding=0,1")
	}
	if pk.Header != "" {
		args = append(args, "--header="+pk.Header)
	}
	if pk.Multi {
		args = append(args, "--multi")
	}
	if len(pk.Expect) > 0 {
		args = append(args, "--expect="+strings.Join(pk.Expect, ","))
	}
	return args
}

func Preview(argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		if a == tmt.PickKey {
			parts[i] = "{1}"
		} else {
			parts[i] = quote(a)
		}
	}
	return strings.Join(parts, " ")
}

func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func Parse(out string, pk tmt.Pick) tmt.Picked {
	var lines []string
	if out != "" {
		lines = strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	}
	var picked tmt.Picked
	if len(pk.Expect) > 0 && len(lines) > 0 {
		picked.Key = lines[0]
		lines = lines[1:]
	}
	byKey := make(map[string]tmt.PickItem, len(pk.Items))
	for _, it := range pk.Items {
		if _, ok := byKey[it.Key]; !ok {
			byKey[it.Key] = it
		}
	}
	for _, line := range lines {
		if line == "" {
			continue
		}
		key, display, _ := strings.Cut(line, "\t")
		it, ok := byKey[key]
		if !ok {
			it = tmt.PickItem{Key: key, Display: display}
		}
		picked.Items = append(picked.Items, it)
	}
	return picked
}
