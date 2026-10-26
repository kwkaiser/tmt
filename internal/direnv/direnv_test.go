package direnv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fake(t *testing.T, script string) (bin, log string) {
	t.Helper()
	tmp := t.TempDir()
	bin = filepath.Join(tmp, "direnv")
	log = filepath.Join(tmp, "args")
	body := "#!/bin/sh\nprintf '%s\\n' \"$@\" > '" + log + "'\n" + script
	if err := os.WriteFile(bin, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, log
}

func envrc(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".envrc"), []byte("use flake\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestAllow(t *testing.T) {
	bin, log := fake(t, "")
	dir := envrc(t)
	if err := (&Direnv{Bin: bin}).Allow(t.Context(), dir); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if want := "allow\n" + dir + "\n"; string(got) != want {
		t.Fatalf("args = %q; want %q", got, want)
	}
}

func TestAllowNoEnvrc(t *testing.T) {
	bin, log := fake(t, "")
	if err := (&Direnv{Bin: bin}).Allow(t.Context(), t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatalf("direnv was executed without .envrc: %v", err)
	}
}

func TestAllowEnvrcDirectory(t *testing.T) {
	bin, log := fake(t, "")
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".envrc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := (&Direnv{Bin: bin}).Allow(t.Context(), dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatalf("direnv was executed for non-file .envrc: %v", err)
	}
}

func TestAllowMissingBinary(t *testing.T) {
	d := &Direnv{Bin: filepath.Join(t.TempDir(), "missing")}
	if err := d.Allow(t.Context(), envrc(t)); err != nil {
		t.Fatalf("Allow = %v; want nil", err)
	}
}

func TestAllowFailure(t *testing.T) {
	bin, _ := fake(t, "echo 'permission denied' >&2\nexit 1\n")
	err := (&Direnv{Bin: bin}).Allow(t.Context(), envrc(t))
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("Allow = %v; want error containing stderr", err)
	}
}

func TestNewDefaultBin(t *testing.T) {
	if b := New().bin(); b != "direnv" {
		t.Fatalf("bin = %q; want direnv", b)
	}
	if b := (&Direnv{}).bin(); b != "direnv" {
		t.Fatalf("empty bin = %q; want direnv", b)
	}
}
