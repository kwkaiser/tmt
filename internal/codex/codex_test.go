package codex

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

func newTestAgent(codexHome, home string) *Agent {
	return &Agent{
		Cmd: "codex",
		Getenv: func(k string) string {
			if k == "CODEX_HOME" {
				return codexHome
			}
			return ""
		},
		HomeDir: func() (string, error) { return home, nil },
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestTrustCreatesConfig(t *testing.T) {
	home := t.TempDir()
	if err := newTestAgent("", home).Trust(context.Background(), `/tasks/a "b"`); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".codex", "config.toml")
	if got, want := read(t, path), "[projects.\"/tasks/a \\\"b\\\"\"]\ntrust_level = \"trusted\"\n"; got != want {
		t.Fatalf("config = %q; want %q", got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", info.Mode().Perm())
	}
}

func TestTrustAppendsAndIsIdempotent(t *testing.T) {
	codexHome := t.TempDir()
	path := filepath.Join(codexHome, "config.toml")
	if err := os.WriteFile(path, []byte("model = \"gpt-5\""), 0o640); err != nil {
		t.Fatal(err)
	}
	a := newTestAgent(codexHome, t.TempDir())
	for range 2 {
		if err := a.Trust(context.Background(), "/tasks/x"); err != nil {
			t.Fatal(err)
		}
	}
	want := "model = \"gpt-5\"\n\n[projects.\"/tasks/x\"]\ntrust_level = \"trusted\"\n"
	if got := read(t, path); got != want {
		t.Fatalf("config = %q; want %q", got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %v", info.Mode().Perm())
	}
}

func TestTrustRespectsExistingEntry(t *testing.T) {
	codexHome := t.TempDir()
	path := filepath.Join(codexHome, "config.toml")
	body := "[projects.'/tasks/x']\ntrust_level = \"untrusted\"\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := newTestAgent(codexHome, t.TempDir()).Trust(context.Background(), "/tasks/x"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, path); got != body {
		t.Fatalf("config = %q; want unchanged", got)
	}
}

func TestWriteAndReadContext(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "api", ".env.mcp"), []byte("A=1"), 0o600); err != nil {
		t.Fatal(err)
	}
	task := tmt.Task{Dir: dir, Topic: "work-x", Base: "/base", Branch: "u/k/x-1"}
	a := newTestAgent("", t.TempDir())
	if err := a.WriteContext(context.Background(), task, []string{"api"}); err != nil {
		t.Fatal(err)
	}
	text, err := a.ReadContext(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(text, "# Task: work-x\n") || !strings.Contains(text, "- api\n") {
		t.Fatalf("context = %q", text)
	}
	if got := read(t, filepath.Join(dir, ".env.mcp")); got != "A=1\n" {
		t.Fatalf(".env.mcp = %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Fatalf("CLAUDE.md should not exist: %v", err)
	}
	if got, want := a.Command(dir), "(set -a && . ./.env.mcp && set +a && exec codex)"; got != want {
		t.Fatalf("Command = %q; want %q", got, want)
	}
}
