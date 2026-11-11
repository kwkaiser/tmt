package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"

	"git.kwkaiser.io/kwkaiser/tmt/internal/taskfiles"
	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

const (
	contextFile = "CLAUDE.md"
	mcpFile     = ".mcp.json"
	configFile  = ".claude.json"
)

type Agent struct {
	Cmd     string
	Getenv  func(string) string
	HomeDir func() (string, error)
}

var _ tmt.Agent = (*Agent)(nil)

func New(command string) *Agent {
	return &Agent{Cmd: command, Getenv: os.Getenv, HomeDir: os.UserHomeDir}
}

func (a *Agent) Command(dir string) string {
	return taskfiles.Command(dir, a.Cmd)
}

func (a *Agent) configPath() (string, error) {
	dir := a.Getenv("CLAUDE_CONFIG_DIR")
	if dir == "" {
		home, err := a.HomeDir()
		if err != nil {
			return "", err
		}
		dir = home
	}
	return filepath.Join(dir, configFile), nil
}

func (a *Agent) Trust(ctx context.Context, dir string) error {
	primary, err := a.configPath()
	if err != nil {
		return err
	}
	if err := trustFile(primary, dir, true); err != nil {
		return err
	}
	others, err := a.otherConfigPaths()
	if err != nil {
		return err
	}
	seen := map[string]bool{taskfiles.Resolve(primary): true}
	for _, p := range others {
		r := taskfiles.Resolve(p)
		if seen[r] {
			continue
		}
		seen[r] = true
		if err := trustFile(p, dir, false); err != nil {
			return err
		}
	}
	return nil
}

func (a *Agent) otherConfigPaths() ([]string, error) {
	home, err := a.HomeDir()
	if err != nil {
		return nil, err
	}
	dirs, err := filepath.Glob(filepath.Join(home, ".claude*", configFile))
	if err != nil {
		return nil, err
	}
	return append([]string{filepath.Join(home, configFile)}, dirs...), nil
}

func trustFile(path, dir string, create bool) error {
	path = taskfiles.Resolve(path)
	mode := fs.FileMode(0o600)
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if !create {
			return nil
		}
		data = []byte("{}")
	case err != nil:
		return err
	default:
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		mode = info.Mode().Perm()
	}
	cfg, err := decodeObject(data)
	if err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	if cfg == nil {
		cfg = map[string]any{}
	}
	projects, err := childObject(cfg, "projects")
	if err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	project, err := childObject(projects, dir)
	if err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	if accepted, ok := project["hasTrustDialogAccepted"].(bool); ok && accepted {
		return nil
	}
	project["hasTrustDialogAccepted"] = true
	out, err := encode(cfg)
	if err != nil {
		return err
	}
	return taskfiles.WriteAtomic(path, out, mode)
}

func childObject(parent map[string]any, key string) (map[string]any, error) {
	switch v := parent[key].(type) {
	case nil:
		m := map[string]any{}
		parent[key] = m
		return m, nil
	case map[string]any:
		return v, nil
	default:
		return nil, fmt.Errorf("%q is not an object", key)
	}
}

func decodeObject(data []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return nil, err
	}
	return m, nil
}

func encode(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (a *Agent) WriteContext(ctx context.Context, t tmt.Task, worktrees []string) error {
	if err := os.WriteFile(filepath.Join(t.Dir, contextFile), []byte(taskfiles.RenderContext(t, worktrees)), 0o644); err != nil {
		return err
	}
	if err := writeMCP(t.Dir); err != nil {
		return err
	}
	return taskfiles.WriteEnvFiles(t.Dir)
}

func writeMCP(dir string) error {
	configs, err := taskfiles.RepoFiles(dir, mcpFile)
	if err != nil {
		return err
	}
	out := filepath.Join(dir, mcpFile)
	if len(configs) == 0 {
		if err := os.Remove(out); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	servers := map[string]any{}
	for _, path := range configs {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		cfg, err := decodeObject(data)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		switch s := cfg["mcpServers"].(type) {
		case nil:
		case map[string]any:
			maps.Copy(servers, s)
		default:
			return fmt.Errorf("parse %s: mcpServers is not an object", path)
		}
	}
	data, err := encode(map[string]any{"mcpServers": servers})
	if err != nil {
		return err
	}
	return os.WriteFile(out, data, 0o644)
}

func (a *Agent) ReadContext(ctx context.Context, t tmt.Task) (string, error) {
	data, err := os.ReadFile(filepath.Join(t.Dir, contextFile))
	if err != nil {
		return "", err
	}
	return string(data), nil
}
