package devshell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

const shellFlake = `{ outputs = _: { devShells = { }; }; }`

func lock(modified string) string {
	return `{"root":"root","nodes":{"root":{"inputs":{"nixpkgs":"nixpkgs_2"}},"nixpkgs_2":{"locked":{"lastModified":` + modified + `}}}}`
}

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
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

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func TestWriteMergesFlakes(t *testing.T) {
	task := t.TempDir()
	shared := t.TempDir()

	write(t, filepath.Join(task, "api", "flake.nix"), shellFlake)
	write(t, filepath.Join(task, "api", "flake.lock"), lock("100"))

	write(t, filepath.Join(shared, "web", "flake.nix"), shellFlake)
	write(t, filepath.Join(shared, "web", "flake.lock"), lock("200"))
	if err := os.Symlink(filepath.Join(shared, "web"), filepath.Join(task, "web")); err != nil {
		t.Fatal(err)
	}

	write(t, filepath.Join(task, "nolock", "flake.nix"), shellFlake)
	write(t, filepath.Join(task, "cfg", "flake.nix"), `{ outputs = _: { nixosConfigurations = { }; }; }`)
	write(t, filepath.Join(task, "cfg", ".envrc"), "use flake\ndotenv_if_exists .env\n")
	write(t, filepath.Join(task, "plain", ".envrc"), "dotenv\ndotenv_if_exists conf/dev.env\n")
	write(t, filepath.Join(task, ".hidden", "flake.nix"), shellFlake)

	if err := New().Write(t.Context(), tmt.Task{Dir: task}); err != nil {
		t.Fatal(err)
	}

	realTask, err := filepath.EvalSymlinks(task)
	if err != nil {
		t.Fatal(err)
	}
	flake := read(t, filepath.Join(task, FlakeDir, "flake.nix"))
	for _, want := range []string{
		`"api" = {` + "\n      url = \"git+file://" + realTask + "/api\";\n    };",
		`"nolock" = {` + "\n      url = \"git+file://" + realTask + "/nolock\";\n    };",
		`nixpkgs = inputs."api".inputs.nixpkgs;`,
	} {
		if !strings.Contains(flake, want) {
			t.Errorf("flake.nix missing %q:\n%s", want, flake)
		}
	}
	for _, unwanted := range []string{`"web"`, `"cfg"`, `"plain"`, `".hidden"`} {
		if strings.Contains(flake, unwanted) {
			t.Errorf("flake.nix contains %s:\n%s", unwanted, flake)
		}
	}

	want := strings.Join([]string{
		"watch_file api/flake.nix api/flake.lock",
		"watch_file nolock/flake.nix nolock/flake.lock",
		"use flake ./" + FlakeDir + " --recreate-lock-file",
		"dotenv_if_exists cfg/.env",
		"dotenv_if_exists plain/.env",
		"dotenv_if_exists plain/conf/dev.env",
	}, "\n") + "\n"
	if got := read(t, filepath.Join(task, ".envrc")); got != want {
		t.Fatalf(".envrc = %q; want %q", got, want)
	}
}

func TestWriteWithoutNixpkgsSkipsFlake(t *testing.T) {
	task := t.TempDir()
	write(t, filepath.Join(task, "api", "flake.nix"), shellFlake)
	write(t, filepath.Join(task, "api", ".envrc"), "dotenv_if_exists\n")
	write(t, filepath.Join(task, FlakeDir, "flake.nix"), "stale")

	if err := New().Write(t.Context(), tmt.Task{Dir: task}); err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(task, FlakeDir)) {
		t.Fatal("stale flake dir was kept")
	}
	if got := read(t, filepath.Join(task, ".envrc")); got != "dotenv_if_exists api/.env\n" {
		t.Fatalf(".envrc = %q", got)
	}
}

func TestWriteRemovesStaleFiles(t *testing.T) {
	task := t.TempDir()
	write(t, filepath.Join(task, FlakeDir, "flake.nix"), "stale")
	write(t, filepath.Join(task, ".envrc"), "stale")
	write(t, filepath.Join(task, "api", "main.go"), "package main")

	if err := New().Write(t.Context(), tmt.Task{Dir: task}); err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(task, FlakeDir)) || exists(filepath.Join(task, ".envrc")) {
		t.Fatal("stale files were kept")
	}
}

func TestWriteUnchangedKeepsMtime(t *testing.T) {
	task := t.TempDir()
	write(t, filepath.Join(task, "api", "flake.nix"), shellFlake)
	write(t, filepath.Join(task, "api", "flake.lock"), lock("1"))
	if err := New().Write(t.Context(), tmt.Task{Dir: task}); err != nil {
		t.Fatal(err)
	}
	old := time.Unix(1, 0)
	paths := []string{filepath.Join(task, FlakeDir, "flake.nix"), filepath.Join(task, ".envrc")}
	for _, p := range paths {
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	if err := New().Write(t.Context(), tmt.Task{Dir: task}); err != nil {
		t.Fatal(err)
	}
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if !info.ModTime().Equal(old) {
			t.Errorf("%s was rewritten", p)
		}
	}
}

func TestNixString(t *testing.T) {
	if got, want := nixString(`a"b\c${d}`), `"a\"b\\c\${d}"`; got != want {
		t.Fatalf("nixString = %s; want %s", got, want)
	}
}

func TestShellQuote(t *testing.T) {
	for in, want := range map[string]string{
		"api/.env":  "api/.env",
		"my repo":   "'my repo'",
		"it's/.env": `'it'\''s/.env'`,
	} {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q; want %q", in, got, want)
		}
	}
}
