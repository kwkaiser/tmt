package codex

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"git.kwkaiser.io/kwkaiser/tmt/internal/taskfiles"
	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

const (
	contextFile = "AGENTS.md"
	configFile  = "config.toml"
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
	dir := a.Getenv("CODEX_HOME")
	if dir == "" {
		home, err := a.HomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".codex")
	}
	return filepath.Join(dir, configFile), nil
}

func (a *Agent) Trust(ctx context.Context, dir string) error {
	path, err := a.configPath()
	if err != nil {
		return err
	}
	path = taskfiles.Resolve(path)
	mode := fs.FileMode(0o600)
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
	case err != nil:
		return err
	default:
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		mode = info.Mode().Perm()
	}
	if hasProject(data, dir) {
		return nil
	}
	var buf bytes.Buffer
	buf.Write(data)
	if len(data) > 0 {
		if data[len(data)-1] != '\n' {
			buf.WriteByte('\n')
		}
		buf.WriteByte('\n')
	}
	buf.WriteString(projectHeader(dir) + "\ntrust_level = \"trusted\"\n")
	return taskfiles.WriteAtomic(path, buf.Bytes(), mode)
}

func projectHeader(dir string) string {
	return "[projects." + quote(dir) + "]"
}

func quote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + r.Replace(s) + `"`
}

func hasProject(data []byte, dir string) bool {
	want := []string{projectHeader(dir), "[projects.'" + dir + "']"}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		if slices.Contains(want, strings.TrimSpace(sc.Text())) {
			return true
		}
	}
	return false
}

func (a *Agent) WriteContext(ctx context.Context, t tmt.Task, worktrees []string) error {
	if err := os.WriteFile(filepath.Join(t.Dir, contextFile), []byte(taskfiles.RenderContext(t, worktrees)), 0o644); err != nil {
		return err
	}
	return taskfiles.WriteEnvFiles(t.Dir)
}

func (a *Agent) ReadContext(ctx context.Context, t tmt.Task) (string, error) {
	data, err := os.ReadFile(filepath.Join(t.Dir, contextFile))
	if err != nil {
		return "", err
	}
	return string(data), nil
}
