package fzf

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

var common = []string{
	"--ansi", "--layout=reverse", "--info=inline-right", "--highlight-line",
	"--prompt=❯ ", "--pointer=▌", "--gutter= ", "--marker=▍", "--separator=─", "--scrollbar=│",
}

func TestArgsPopup(t *testing.T) {
	got := Args(tmt.Pick{
		Label:   "my-topic",
		Header:  ":: <enter> jump",
		Preview: []string{"/bin/tmt", "_preview", "entry", "/tmp/x", tmt.PickKey},
		Expect:  []string{"ctrl-a", "ctrl-t"},
	}, "fg:1,bg:2", true)
	want := append(append([]string{}, common...),
		"--color=fg:1,bg:2",
		`--delimiter=\t`, "--with-nth=2..",
		"--preview='/bin/tmt' '_preview' 'entry' '/tmp/x' {1}",
		"--preview-window=right,55%,border-left,<60(down,50%,border-top)",
		"--border=none", "--input-border=line", "--input-label= my-topic ", "--no-separator",
		"--header=:: <enter> jump",
		"--expect=ctrl-a,ctrl-t",
	)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got\n%q\nwant\n%q", got, want)
	}
}

func TestArgsFramed(t *testing.T) {
	got := Args(tmt.Pick{Label: "Base", Multi: true}, "", false)
	want := append(append([]string{}, common...),
		`--delimiter=\t`, "--with-nth=2..",
		"--preview-window=right,55%,border-left,<60(down,50%,border-top)",
		"--border=rounded", "--border-label= Base ", "--padding=0,1",
		"--multi",
	)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got\n%q\nwant\n%q", got, want)
	}
}

func TestPreview(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		want string
	}{
		{"plain", []string{"tmt", "_preview", "task", tmt.PickKey, ""}, "'tmt' '_preview' 'task' {1} ''"},
		{"quote and space", []string{"/Users/o'brien/my dir/tmt", "repo", tmt.PickKey}, `'/Users/o'\''brien/my dir/tmt' 'repo' {1}`},
		{"metachars", []string{"$(rm -rf /)", "`x`", "a;b|c", tmt.PickKey}, "'$(rm -rf /)' '`x`' 'a;b|c' {1}"},
		{"brace literal", []string{"{1}", tmt.PickKey}, "'{1}' {1}"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Preview(tc.argv); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPreviewRoundTrip(t *testing.T) {
	argv := []string{"printf", `%s\n`, "/a/o'brien's dir", "", "$HOME", "'", tmt.PickKey}
	key := "it's a key"
	script := strings.ReplaceAll(Preview(argv), "{1}", quote(key))
	out, err := exec.Command("sh", "-c", script).Output()
	if err != nil {
		t.Fatal(err)
	}
	want := "/a/o'brien's dir\n\n$HOME\n'\nit's a key\n"
	if string(out) != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

func TestInput(t *testing.T) {
	got := Input([]tmt.PickItem{{Key: "/a b", Display: "x  y"}, {Key: "c", Display: "\x1b[1mz\x1b[0m"}})
	want := "/a b\tx  y\nc\t\x1b[1mz\x1b[0m\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestParse(t *testing.T) {
	items := []tmt.PickItem{
		{Key: "claude", Display: "root"},
		{Key: "api", Display: "api main"},
		{Key: "web", Display: "web\tfeature"},
	}
	tests := []struct {
		name string
		pick tmt.Pick
		out  string
		want tmt.Picked
	}{
		{"single", tmt.Pick{Items: items}, "api\tapi main\n", tmt.Picked{Items: []tmt.PickItem{items[1]}}},
		{"empty", tmt.Pick{Items: items}, "", tmt.Picked{}},
		{"expect enter", tmt.Pick{Items: items, Expect: []string{"ctrl-a"}}, "\nclaude\troot\n", tmt.Picked{Items: []tmt.PickItem{items[0]}}},
		{"expect key", tmt.Pick{Items: items, Expect: []string{"ctrl-a", "ctrl-t"}}, "ctrl-a\napi\tapi main\n", tmt.Picked{Key: "ctrl-a", Items: []tmt.PickItem{items[1]}}},
		{"expect key only", tmt.Pick{Items: items, Expect: []string{"ctrl-t"}}, "ctrl-t\n", tmt.Picked{Key: "ctrl-t"}},
		{"multi", tmt.Pick{Items: items, Multi: true}, "web\tweb\tfeature\napi\tapi main\n", tmt.Picked{Items: []tmt.PickItem{items[2], items[1]}}},
		{"unknown key", tmt.Pick{Items: items}, "zzz\tdisp\n", tmt.Picked{Items: []tmt.PickItem{{Key: "zzz", Display: "disp"}}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Parse(tc.out, tc.pick); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

func fake(t *testing.T, out string, code int) (*Picker, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "out"), []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "fzf")
	script := "#!/bin/sh\n" +
		"d=$(dirname \"$0\")\n" +
		"cat > \"$d/stdin\"\n" +
		"printf '%s\\n' \"$@\" > \"$d/args\"\n" +
		"cat \"$d/out\"\n" +
		"exit " + strconv.Itoa(code) + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	p := New("", false)
	p.Bin = bin
	return p, dir
}

func TestPickExitCodes(t *testing.T) {
	items := []tmt.PickItem{{Key: "a", Display: "A"}, {Key: "b", Display: "B"}}
	tests := []struct {
		name    string
		code    int
		out     string
		ok      bool
		wantErr bool
		want    tmt.Picked
	}{
		{"selected", 0, "ctrl-a\nb\tB\n", true, false, tmt.Picked{Key: "ctrl-a", Items: []tmt.PickItem{items[1]}}},
		{"no match", 1, "", false, false, tmt.Picked{}},
		{"aborted", 130, "", false, false, tmt.Picked{}},
		{"error", 2, "", false, true, tmt.Picked{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, dir := fake(t, tc.out, tc.code)
			got, ok, err := p.Pick(context.Background(), tmt.Pick{Label: "L", Items: items, Expect: []string{"ctrl-a"}})
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if ok != tc.ok || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got (%#v, %v), want (%#v, %v)", got, ok, tc.want, tc.ok)
			}
			stdin, err := os.ReadFile(filepath.Join(dir, "stdin"))
			if err != nil {
				t.Fatal(err)
			}
			if string(stdin) != "a\tA\nb\tB\n" {
				t.Fatalf("stdin = %q", stdin)
			}
			args, err := os.ReadFile(filepath.Join(dir, "args"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(args), "--expect=ctrl-a\n") || !strings.Contains(string(args), "--border-label= L \n") {
				t.Fatalf("args = %q", args)
			}
		})
	}
}

func TestPickContextCanceled(t *testing.T) {
	p, _ := fake(t, "", 0)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, ok, err := p.Pick(ctx, tmt.Pick{}); err == nil || ok {
		t.Fatalf("got ok=%v err=%v", ok, err)
	}
}
