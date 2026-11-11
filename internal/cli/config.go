package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

var defaultAgentCommands = map[string]string{
	"claude": "ccp",
	"codex":  "codex",
}

type backendConfig struct {
	FZFColors     string
	Popup         bool
	Agent         string
	AgentCommands map[string]string
	AgentDirs     map[string]string
}

type fileConfig struct {
	Agent  string            `yaml:"agent"`
	Agents map[string]string `yaml:"agents"`
	Dirs   map[string]string `yaml:"dirs"`
}

func envOr(getenv func(string) string, name, fallback string) string {
	if v := getenv(name); v != "" {
		return v
	}
	return fallback
}

func configPath(getenv func(string) string) string {
	if p := getenv("TMT_CONFIG"); p != "" {
		return p
	}
	dir := getenv("XDG_CONFIG_HOME")
	if dir == "" {
		dir = filepath.Join(getenv("HOME"), ".config")
	}
	return filepath.Join(dir, "tmt", "config.yaml")
}

func readFileConfig(path string, readFile func(string) ([]byte, error)) (fileConfig, error) {
	var fc fileConfig
	data, err := readFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return fc, nil
	}
	if err != nil {
		return fc, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&fc); err != nil && !errors.Is(err, io.EOF) {
		return fc, fmt.Errorf("parse %s: %w", path, err)
	}
	return fc, nil
}

func expandHome(home, path string) string {
	if path == "~" {
		return home
	}
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		return filepath.Join(home, rest)
	}
	return path
}

func checkAgent(name, where string) error {
	if _, ok := defaultAgentCommands[name]; ok {
		return nil
	}
	known := slices.Sorted(maps.Keys(defaultAgentCommands))
	return fmt.Errorf("%s: unknown agent %q (known: %s)", where, name, strings.Join(known, ", "))
}

func loadAgents(getenv func(string) string, readFile func(string) ([]byte, error)) (string, map[string]string, map[string]string, error) {
	path := configPath(getenv)
	fc, err := readFileConfig(path, readFile)
	if err != nil {
		return "", nil, nil, err
	}
	commands := maps.Clone(defaultAgentCommands)
	for name, cmd := range fc.Agents {
		if err := checkAgent(name, path); err != nil {
			return "", nil, nil, err
		}
		if cmd != "" {
			commands[name] = cmd
		}
	}
	if v := getenv("TMT_CLAUDE_COMMAND"); v != "" {
		commands["claude"] = v
	}
	agent := fc.Agent
	if agent == "" {
		agent = "claude"
	}
	agent = envOr(getenv, "TMT_AGENT", agent)
	if err := checkAgent(agent, "default agent"); err != nil {
		return "", nil, nil, err
	}
	dirs := make(map[string]string, len(fc.Dirs))
	for dir, name := range fc.Dirs {
		if err := checkAgent(name, path); err != nil {
			return "", nil, nil, err
		}
		abs := expandHome(getenv("HOME"), dir)
		if !filepath.IsAbs(abs) {
			return "", nil, nil, fmt.Errorf("%s: dir %q must be absolute or start with ~/", path, dir)
		}
		dirs[filepath.Clean(abs)] = name
	}
	return agent, commands, dirs, nil
}

func loadConfig(getenv func(string) string, executable func() (string, error), readFile func(string) ([]byte, error)) (tmt.Config, backendConfig, error) {
	self, err := executable()
	if err != nil {
		return tmt.Config{}, backendConfig{}, err
	}
	cfg := tmt.Config{
		Root:         envOr(getenv, "TMT_ROOT", getenv("HOME")+"/tasks"),
		BranchPrefix: envOr(getenv, "TMT_BRANCH_PREFIX", "u/kwkaiser/"),
		CloneNames:   strings.Fields(envOr(getenv, "TMT_CLONE", "node_modules .env .env.local .env.mcp")),
		Self:         self,
		Theme: tmt.Theme{
			Worktree: envOr(getenv, "TMT_COLOR_WORKTREE", "38;5;114"),
			Main:     envOr(getenv, "TMT_COLOR_MAIN", "38;5;244"),
			Root:     envOr(getenv, "TMT_COLOR_ROOT", "38;5;75"),
			Accent:   envOr(getenv, "TMT_COLOR_ACCENT", "38;5;221"),
			Dim:      envOr(getenv, "TMT_COLOR_DIM", "38;5;246"),
		},
	}
	agent, commands, dirs, err := loadAgents(getenv, readFile)
	if err != nil {
		return tmt.Config{}, backendConfig{}, err
	}
	backends := backendConfig{
		FZFColors:     getenv("TMT_FZF_COLORS"),
		Popup:         getenv("TMT_POPUP") != "",
		Agent:         agent,
		AgentCommands: commands,
		AgentDirs:     dirs,
	}
	return cfg, backends, nil
}
