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
	"strings"

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

func (a *Agent) Command() string {
	return a.Cmd
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
	path, err := a.configPath()
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	mode := fs.FileMode(0o600)
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
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
	return writeAtomic(path, out, mode)
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

func writeAtomic(path string, data []byte, mode fs.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

func (a *Agent) WriteContext(ctx context.Context, t tmt.Task, worktrees []string) error {
	if err := os.WriteFile(filepath.Join(t.Dir, contextFile), []byte(renderContext(t, worktrees)), 0o644); err != nil {
		return err
	}
	return writeMCP(t.Dir)
}

func renderContext(t tmt.Task, worktrees []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Task: %s\n\n", t.Topic)
	fmt.Fprintf(&b, "This directory mirrors %s.\n", t.Base)
	fmt.Fprintf(&b, "These repos are git worktrees on branch `%s` and are the only ones you may modify:\n\n", t.Branch)
	for _, r := range worktrees {
		fmt.Fprintf(&b, "- %s\n", r)
	}
	b.WriteString("\nEvery other directory is a symlink to the shared main checkout and is read-only.\n")
	fmt.Fprintf(&b, "To modify one, first run `tmt add -t %s <repo>`, which converts it into a worktree for this task.\n", t.Topic)
	return b.String()
}

func mcpConfigs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var configs []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		path := filepath.Join(dir, e.Name(), mcpFile)
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		configs = append(configs, path)
	}
	return configs, nil
}

func writeMCP(dir string) error {
	configs, err := mcpConfigs(dir)
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
