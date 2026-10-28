package fzf

import (
	"context"
	"fmt"
	"strings"

	fzf "github.com/junegunn/fzf/src"

	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

type Runner func(args, input []string) (int, []string, error)

type Picker struct {
	Run    Runner
	Colors string
	Popup  bool
}

var _ tmt.Picker = (*Picker)(nil)

func New(colors string, popup bool) *Picker {
	return &Picker{Run: Library, Colors: colors, Popup: popup}
}

func Library(args, input []string) (int, []string, error) {
	opts, err := fzf.ParseOptions(true, args)
	if err != nil {
		return fzf.ExitError, nil, err
	}
	in := make(chan string, len(input))
	for _, l := range input {
		in <- l
	}
	close(in)
	out := make(chan string)
	opts.Input = in
	opts.Output = out

	var lines []string
	done := make(chan struct{})
	go func() {
		for l := range out {
			lines = append(lines, l)
		}
		close(done)
	}()
	code, err := fzf.Run(opts)
	close(out)
	<-done
	return code, lines, err
}

func (p *Picker) Pick(ctx context.Context, pk tmt.Pick) (tmt.Picked, bool, error) {
	if err := ctx.Err(); err != nil {
		return tmt.Picked{}, false, err
	}
	run := p.Run
	if run == nil {
		run = Library
	}
	code, out, err := run(Args(pk, p.Colors, p.Popup), Input(pk.Items))
	if ctx.Err() != nil {
		return tmt.Picked{}, false, ctx.Err()
	}
	switch code {
	case fzf.ExitOk:
	case fzf.ExitNoMatch, fzf.ExitInterrupt:
		return tmt.Picked{}, false, nil
	default:
		if err == nil {
			err = fmt.Errorf("exit %d", code)
		}
		return tmt.Picked{}, false, fmt.Errorf("fzf: %w", err)
	}
	return Parse(out, pk), true, nil
}

func Input(items []tmt.PickItem) []string {
	lines := make([]string, len(items))
	for i, it := range items {
		lines[i] = it.Key + "\t" + it.Display
	}
	return lines
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

func Parse(lines []string, pk tmt.Pick) tmt.Picked {
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
