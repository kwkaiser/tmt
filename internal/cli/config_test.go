package cli

import (
	"errors"
	"io/fs"
	"reflect"
	"testing"

	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

func env(kv map[string]string) func(string) string {
	return func(k string) string { return kv[k] }
}

func executable() (string, error) { return "/bin/tmt", nil }

func noFile(string) ([]byte, error) { return nil, fs.ErrNotExist }

func TestConfigDefaults(t *testing.T) {
	cfg, backends, err := loadConfig(env(map[string]string{"HOME": "/home/k"}), executable, noFile)
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
	wantBackends := backendConfig{
		Agent:         "claude",
		AgentCommands: map[string]string{"claude": "ccp", "codex": "codex"},
		AgentDirs:     map[string]string{},
	}
	if want := wantBackends; !reflect.DeepEqual(backends, want) {
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
		"TMT_AGENT":          "codex",
	}), executable, noFile)
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
	wantBackends := backendConfig{
		FZFColors:     "fg:#ffffff,bg:-1",
		Popup:         true,
		Agent:         "codex",
		AgentCommands: map[string]string{"claude": "claude --resume", "codex": "codex"},
		AgentDirs:     map[string]string{},
	}
	if !reflect.DeepEqual(backends, wantBackends) {
		t.Fatalf("backends = %#v; want %#v", backends, wantBackends)
	}
}

func TestConfigExecutableError(t *testing.T) {
	_, _, err := loadConfig(env(nil), func() (string, error) { return "", errors.New("no exe") }, noFile)
	if err == nil || err.Error() != "no exe" {
		t.Fatalf("err = %v", err)
	}
}

func files(m map[string]string) func(string) ([]byte, error) {
	return func(p string) ([]byte, error) {
		if s, ok := m[p]; ok {
			return []byte(s), nil
		}
		return nil, fs.ErrNotExist
	}
}

func TestConfigFileAgents(t *testing.T) {
	read := files(map[string]string{"/home/k/.config/tmt/config.yaml": `
agent: codex
agents:
  claude: ccpm
dirs:
  ~/work: claude
  /srv/code/: codex
`})
	_, backends, err := loadConfig(env(map[string]string{"HOME": "/home/k"}), executable, read)
	if err != nil {
		t.Fatal(err)
	}
	want := backendConfig{
		Agent:         "codex",
		AgentCommands: map[string]string{"claude": "ccpm", "codex": "codex"},
		AgentDirs:     map[string]string{"/home/k/work": "claude", "/srv/code": "codex"},
	}
	if !reflect.DeepEqual(backends, want) {
		t.Fatalf("backends = %#v; want %#v", backends, want)
	}
}

func TestConfigFilePath(t *testing.T) {
	for _, tc := range []struct {
		env  map[string]string
		path string
	}{
		{map[string]string{"HOME": "/home/k"}, "/home/k/.config/tmt/config.yaml"},
		{map[string]string{"HOME": "/home/k", "XDG_CONFIG_HOME": "/xdg"}, "/xdg/tmt/config.yaml"},
		{map[string]string{"HOME": "/home/k", "XDG_CONFIG_HOME": "/xdg", "TMT_CONFIG": "/etc/tmt.yaml"}, "/etc/tmt.yaml"},
	} {
		if got := configPath(env(tc.env)); got != tc.path {
			t.Errorf("configPath(%v) = %q; want %q", tc.env, got, tc.path)
		}
	}
}

func TestConfigEnvOverridesFile(t *testing.T) {
	read := files(map[string]string{"/home/k/.config/tmt/config.yaml": "agent: codex\nagents:\n  claude: ccpm\n"})
	_, backends, err := loadConfig(env(map[string]string{
		"HOME":               "/home/k",
		"TMT_AGENT":          "claude",
		"TMT_CLAUDE_COMMAND": "ccp",
	}), executable, read)
	if err != nil {
		t.Fatal(err)
	}
	if backends.Agent != "claude" || backends.AgentCommands["claude"] != "ccp" {
		t.Fatalf("backends = %#v", backends)
	}
}

func TestConfigFileErrors(t *testing.T) {
	for name, body := range map[string]string{
		"unknown default": "agent: aider\n",
		"unknown command": "agents:\n  aider: aider\n",
		"unknown dir":     "dirs:\n  /x: aider\n",
		"relative dir":    "dirs:\n  work: codex\n",
		"unknown field":   "agnet: codex\n",
	} {
		read := files(map[string]string{"/home/k/.config/tmt/config.yaml": body})
		if _, _, err := loadConfig(env(map[string]string{"HOME": "/home/k"}), executable, read); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	if _, _, err := loadConfig(env(map[string]string{"HOME": "/home/k", "TMT_AGENT": "aider"}), executable, noFile); err == nil {
		t.Error("TMT_AGENT=aider: expected error")
	}
}

func TestConfigEmptyFile(t *testing.T) {
	read := files(map[string]string{"/home/k/.config/tmt/config.yaml": ""})
	_, backends, err := loadConfig(env(map[string]string{"HOME": "/home/k"}), executable, read)
	if err != nil {
		t.Fatal(err)
	}
	if backends.Agent != "claude" {
		t.Fatalf("agent = %q", backends.Agent)
	}
}
