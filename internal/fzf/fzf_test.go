package fzf

import (
	"context"
	"os/exec"
	"reflect"
	"slices"
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
	want := []string{"/a b\tx  y", "c\t\x1b[1mz\x1b[0m"}
	if !reflect.DeepEqual(got, want) {
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
		out  []string
		want tmt.Picked
	}{
		{"single", tmt.Pick{Items: items}, []string{"api\tapi main"}, tmt.Picked{Items: []tmt.PickItem{items[1]}}},
		{"empty", tmt.Pick{Items: items}, nil, tmt.Picked{}},
		{"expect enter", tmt.Pick{Items: items, Expect: []string{"ctrl-a"}}, []string{"", "claude\troot"}, tmt.Picked{Items: []tmt.PickItem{items[0]}}},
		{"expect key", tmt.Pick{Items: items, Expect: []string{"ctrl-a", "ctrl-t"}}, []string{"ctrl-a", "api\tapi main"}, tmt.Picked{Key: "ctrl-a", Items: []tmt.PickItem{items[1]}}},
		{"expect key only", tmt.Pick{Items: items, Expect: []string{"ctrl-t"}}, []string{"ctrl-t"}, tmt.Picked{Key: "ctrl-t"}},
		{"multi", tmt.Pick{Items: items, Multi: true}, []string{"web\tweb\tfeature", "api\tapi main"}, tmt.Picked{Items: []tmt.PickItem{items[2], items[1]}}},
		{"unknown key", tmt.Pick{Items: items}, []string{"zzz\tdisp"}, tmt.Picked{Items: []tmt.PickItem{{Key: "zzz", Display: "disp"}}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Parse(tc.out, tc.pick); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

type call struct {
	args, input []string
}

func fake(out []string, code int) (*Picker, *call) {
	c := &call{}
	p := New("", false)
	p.Run = func(args, input []string) (int, []string, error) {
		c.args, c.input = args, input
		return code, out, nil
	}
	return p, c
}

func TestPickExitCodes(t *testing.T) {
	items := []tmt.PickItem{{Key: "a", Display: "A"}, {Key: "b", Display: "B"}}
	tests := []struct {
		name    string
		code    int
		out     []string
		ok      bool
		wantErr bool
		want    tmt.Picked
	}{
		{"selected", 0, []string{"ctrl-a", "b\tB"}, true, false, tmt.Picked{Key: "ctrl-a", Items: []tmt.PickItem{items[1]}}},
		{"no match", 1, nil, false, false, tmt.Picked{}},
		{"aborted", 130, nil, false, false, tmt.Picked{}},
		{"error", 2, nil, false, true, tmt.Picked{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, c := fake(tc.out, tc.code)
			got, ok, err := p.Pick(context.Background(), tmt.Pick{Label: "L", Items: items, Expect: []string{"ctrl-a"}})
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if ok != tc.ok || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got (%#v, %v), want (%#v, %v)", got, ok, tc.want, tc.ok)
			}
			if !reflect.DeepEqual(c.input, []string{"a\tA", "b\tB"}) {
				t.Fatalf("input = %q", c.input)
			}
			if !slices.Contains(c.args, "--expect=ctrl-a") || !slices.Contains(c.args, "--border-label= L ") {
				t.Fatalf("args = %q", c.args)
			}
		})
	}
}

func TestPickContextCanceled(t *testing.T) {
	p, c := fake(nil, 0)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, ok, err := p.Pick(ctx, tmt.Pick{}); err == nil || ok {
		t.Fatalf("got ok=%v err=%v", ok, err)
	}
	if c.args != nil {
		t.Fatal("fzf ran after cancel")
	}
}

func TestLibraryFilter(t *testing.T) {
	code, out, err := Library([]string{"--filter=b", `--delimiter=\t`, "--with-nth=2.."}, []string{"a\tAlpha", "b\tBravo", "c\tCharlie"})
	if err != nil || code != 0 {
		t.Fatalf("Library = %d, %v", code, err)
	}
	if !reflect.DeepEqual(out, []string{"b\tBravo"}) {
		t.Fatalf("out = %q", out)
	}
}
