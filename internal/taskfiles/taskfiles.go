package taskfiles

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

const (
	MCPEnvFile = ".env.mcp"
	DotEnvFile = ".env"
)

func Command(dir, cmd string) string {
	if info, err := os.Stat(filepath.Join(dir, MCPEnvFile)); err != nil || !info.Mode().IsRegular() {
		return cmd
	}
	return "(set -a && . ./" + MCPEnvFile + " && set +a && exec " + cmd + ")"
}

func RenderContext(t tmt.Task, worktrees []string) string {
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

func RepoFiles(dir, name string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		path := filepath.Join(dir, e.Name(), name)
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		files = append(files, path)
	}
	return files, nil
}

func WriteEnvFiles(dir string) error {
	for _, name := range []string{MCPEnvFile, DotEnvFile} {
		if err := writeEnv(dir, name); err != nil {
			return err
		}
	}
	return nil
}

func writeEnv(dir, name string) error {
	files, err := RepoFiles(dir, name)
	if err != nil {
		return err
	}
	out := filepath.Join(dir, name)
	if len(files) == 0 {
		if err := os.Remove(out); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	var buf bytes.Buffer
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		buf.Write(data)
		if len(data) > 0 && data[len(data)-1] != '\n' {
			buf.WriteByte('\n')
		}
	}
	return WriteAtomic(out, buf.Bytes(), 0o600)
}

func WriteAtomic(path string, data []byte, mode fs.FileMode) error {
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

func Resolve(path string) string {
	if r, err := filepath.EvalSymlinks(path); err == nil {
		return r
	}
	return path
}
