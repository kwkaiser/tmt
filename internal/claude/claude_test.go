package claude

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

func newTestAgent(configDir, home string) *Agent {
	return &Agent{
		Cmd: "ccp",
		Getenv: func(k string) string {
			if k == "CLAUDE_CONFIG_DIR" {
				return configDir
			}
			return ""
		},
		HomeDir: func() (string, error) { return home, nil },
	}
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m, err := decodeObject(data)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func trusted(m map[string]any, dir string) bool {
	projects, _ := m["projects"].(map[string]any)
	project, _ := projects[dir].(map[string]any)
	v, _ := project["hasTrustDialogAccepted"].(bool)
	return v
}

func TestCommand(t *testing.T) {
	if got := New("ccp").Command(); got != "ccp" {
		t.Fatalf("Command = %q; want ccp", got)
	}
}

func TestConfigPath(t *testing.T) {
	a := newTestAgent("", "/home/u")
	if got, _ := a.configPath(); got != "/home/u/.claude.json" {
		t.Fatalf("configPath = %q", got)
	}
	a = newTestAgent("/cfg", "/home/u")
	if got, _ := a.configPath(); got != "/cfg/.claude.json" {
		t.Fatalf("configPath = %q", got)
	}
}

func TestTrustMissingFile(t *testing.T) {
	home := t.TempDir()
	a := newTestAgent("", home)
	if err := a.Trust(context.Background(), "/tasks/foo"); err != nil {
		t.Fatal(err)
	}
	if !trusted(readJSON(t, filepath.Join(home, ".claude.json")), "/tasks/foo") {
		t.Fatal("dir not trusted")
	}
}

func TestTrustAlreadyTrusted(t *testing.T) {
	cfgDir := t.TempDir()
	path := filepath.Join(cfgDir, ".claude.json")
	orig := `{"projects":{"/tasks/foo":{"hasTrustDialogAccepted":true}},"z":1}`
	if err := os.WriteFile(path, []byte(orig), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	a := newTestAgent(cfgDir, t.TempDir())
	if err := a.Trust(context.Background(), "/tasks/foo"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(old) {
		t.Fatalf("mtime changed: %v != %v", info.ModTime(), old)
	}
	data, _ := os.ReadFile(path)
	if string(data) != orig {
		t.Fatalf("content changed: %s", data)
	}
}

func TestTrustPreservesFields(t *testing.T) {
	cfgDir := t.TempDir()
	path := filepath.Join(cfgDir, ".claude.json")
	orig := `{"numStartups":12345678901234567890,"userID":"abc","projects":{"/other":{"hasTrustDialogAccepted":true,"x":[1,2]},"/tasks/foo":{"allowedTools":["a"],"hasTrustDialogAccepted":false}}}`
	if err := os.WriteFile(path, []byte(orig), 0o640); err != nil {
		t.Fatal(err)
	}
	a := newTestAgent(cfgDir, t.TempDir())
	if err := a.Trust(context.Background(), "/tasks/foo"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "12345678901234567890") {
		t.Fatalf("large number lost: %s", data)
	}
	m := readJSON(t, path)
	if !trusted(m, "/tasks/foo") || !trusted(m, "/other") {
		t.Fatalf("trust flags wrong: %s", data)
	}
	if m["userID"] != "abc" {
		t.Fatalf("userID lost: %s", data)
	}
	projects := m["projects"].(map[string]any)
	if tools, _ := projects["/tasks/foo"].(map[string]any)["allowedTools"].([]any); len(tools) != 1 || tools[0] != "a" {
		t.Fatalf("allowedTools lost: %s", data)
	}
	if x, _ := projects["/other"].(map[string]any)["x"].([]any); len(x) != 2 || x[1] != json.Number("2") {
		t.Fatalf("other project lost: %s", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %v; want 0640", info.Mode().Perm())
	}
	entries, _ := os.ReadDir(cfgDir)
	if len(entries) != 1 {
		t.Fatalf("temp files left behind: %v", entries)
	}
}

func TestWriteContextClaudeMD(t *testing.T) {
	dir := t.TempDir()
	task := tmt.Task{Dir: dir, Topic: "foo", Base: "/src", Branch: "u/me/foo-1"}
	if err := New("ccp").WriteContext(context.Background(), task, []string{"api", "web"}); err != nil {
		t.Fatal(err)
	}
	want := "# Task: foo\n" +
		"\n" +
		"This directory mirrors /src.\n" +
		"These repos are git worktrees on branch `u/me/foo-1` and are the only ones you may modify:\n" +
		"\n" +
		"- api\n" +
		"- web\n" +
		"\n" +
		"Every other directory is a symlink to the shared main checkout and is read-only.\n" +
		"To modify one, first run `tmt add -t foo <repo>`, which converts it into a worktree for this task.\n"
	data, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("CLAUDE.md =\n%s\nwant\n%s", data, want)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWriteContextMergesMCP(t *testing.T) {
	dir := t.TempDir()
	shared := t.TempDir()
	writeFile(t, filepath.Join(dir, "a", ".mcp.json"), `{"mcpServers":{"x":{"command":"a-x"},"y":{"command":"a-y"}}}`)
	writeFile(t, filepath.Join(dir, "b", ".mcp.json"), `{"other":true}`)
	writeFile(t, filepath.Join(shared, "c", ".mcp.json"), `{"mcpServers":{"y":{"command":"c-y"},"z":{"command":"c-z","port":12345678901234567890}}}`)
	if err := os.Symlink(filepath.Join(shared, "c"), filepath.Join(dir, "c")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, ".hidden", ".mcp.json"), `{"mcpServers":{"x":{"command":"hidden"}}}`)
	if err := os.MkdirAll(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	task := tmt.Task{Dir: dir, Topic: "foo"}
	if err := New("ccp").WriteContext(context.Background(), task, nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "mcpServers": {
    "x": {
      "command": "a-x"
    },
    "y": {
      "command": "c-y"
    },
    "z": {
      "command": "c-z",
      "port": 12345678901234567890
    }
  }
}
`
	if string(data) != want {
		t.Fatalf(".mcp.json =\n%s\nwant\n%s", data, want)
	}
}

func TestWriteContextRemovesMCP(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".mcp.json"), `{"mcpServers":{}}`)
	if err := os.MkdirAll(filepath.Join(dir, "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	a := New("ccp")
	task := tmt.Task{Dir: dir, Topic: "foo"}
	if err := a.WriteContext(context.Background(), task, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".mcp.json")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("stat .mcp.json: %v; want not exist", err)
	}
	if err := a.WriteContext(context.Background(), task, nil); err != nil {
		t.Fatal(err)
	}
}

func TestWriteContextInvalidMCP(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "b", ".mcp.json")
	writeFile(t, filepath.Join(dir, "a", ".mcp.json"), `{"mcpServers":{}}`)
	writeFile(t, bad, `{"mcpServers":`)
	err := New("ccp").WriteContext(context.Background(), tmt.Task{Dir: dir, Topic: "foo"}, nil)
	if err == nil || !strings.Contains(err.Error(), bad) {
		t.Fatalf("err = %v; want error naming %s", err, bad)
	}
}

func TestReadContext(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "CLAUDE.md"), "hello\n")
	got, err := New("ccp").ReadContext(context.Background(), tmt.Task{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello\n" {
		t.Fatalf("ReadContext = %q", got)
	}
}
