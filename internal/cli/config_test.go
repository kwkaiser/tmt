package cli

import (
	"errors"
	"reflect"
	"testing"

	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

func env(kv map[string]string) func(string) string {
	return func(k string) string { return kv[k] }
}

func executable() (string, error) { return "/bin/tmt", nil }

func TestConfigDefaults(t *testing.T) {
	cfg, backends, err := loadConfig(env(map[string]string{"HOME": "/home/k"}), executable)
	if err != nil {
		t.Fatal(err)
	}
	want := tmt.Config{
		Root:         "/home/k/tasks",
		BranchPrefix: "u/kwkaiser/",
		CloneNames:   []string{"node_modules", ".env", ".env.local", ".env.mcp"},
		Self:         "/bin/tmt",
		Theme: tmt.Theme{
			Worktree: "38;5;114",
			Main:     "38;5;244",
			Root:     "38;5;75",
			Accent:   "38;5;221",
			Dim:      "38;5;246",
		},
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Fatalf("config = %#v; want %#v", cfg, want)
	}
	if want := (backendConfig{ClaudeCommand: "ccp"}); backends != want {
		t.Fatalf("backends = %#v; want %#v", backends, want)
	}
}

func TestConfigEnv(t *testing.T) {
	cfg, backends, err := loadConfig(env(map[string]string{
		"HOME":               "/home/k",
		"TMT_ROOT":           "/srv/tasks",
		"TMT_BRANCH_PREFIX":  "kk/",
		"TMT_CLONE":          "  a\tb\n c ",
		"TMT_COLOR_WORKTREE": "38;2;1;2;3",
		"TMT_COLOR_MAIN":     "38;2;4;5;6",
		"TMT_COLOR_ROOT":     "38;2;7;8;9",
		"TMT_COLOR_ACCENT":   "1",
		"TMT_COLOR_DIM":      "2",
		"TMT_FZF_COLORS":     "fg:#ffffff,bg:-1",
		"TMT_POPUP":          "1",
		"TMT_CLAUDE_COMMAND": "claude --resume",
	}), executable)
	if err != nil {
		t.Fatal(err)
	}
	want := tmt.Config{
		Root:         "/srv/tasks",
		BranchPrefix: "kk/",
		CloneNames:   []string{"a", "b", "c"},
		Self:         "/bin/tmt",
		Theme: tmt.Theme{
			Worktree: "38;2;1;2;3",
			Main:     "38;2;4;5;6",
			Root:     "38;2;7;8;9",
			Accent:   "1",
			Dim:      "2",
		},
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Fatalf("config = %#v; want %#v", cfg, want)
	}
	wantBackends := backendConfig{FZFColors: "fg:#ffffff,bg:-1", Popup: true, ClaudeCommand: "claude --resume"}
	if backends != wantBackends {
		t.Fatalf("backends = %#v; want %#v", backends, wantBackends)
	}
}

func TestConfigExecutableError(t *testing.T) {
	_, _, err := loadConfig(env(nil), func() (string, error) { return "", errors.New("no exe") })
	if err == nil || err.Error() != "no exe" {
		t.Fatalf("err = %v", err)
	}
}
