package trash

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

var _ tmt.Trash = (*Trash)(nil)

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func makeTree(t *testing.T, root string) {
	t.Helper()
	must(t, os.MkdirAll(filepath.Join(root, "node_modules", "pkg"), 0o755))
	must(t, os.WriteFile(filepath.Join(root, "node_modules", "pkg", "index.js"), []byte("x"), 0o644))
}

func bins(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	must(t, err)
	var out []string
	for _, e := range entries {
		out = append(out, filepath.Join(dir, e.Name()))
	}
	return out
}

func TestDiscardMovesTreeIntoBin(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, ".trash")
	src := filepath.Join(base, "wt")
	makeTree(t, src)

	must(t, New(dir).Discard(src))

	if _, err := os.Lstat(src); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("source still present: %v", err)
	}
	b := bins(t, dir)
	if len(b) != 1 {
		t.Fatalf("want 1 bin, got %v", b)
	}
	data, err := os.ReadFile(filepath.Join(b[0], "wt", "node_modules", "pkg", "index.js"))
	must(t, err)
	if string(data) != "x" {
		t.Fatalf("content = %q", data)
	}
}

func TestDiscardSameNameDoesNotCollide(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, ".trash")
	tr := New(dir)
	for i := 0; i < 5; i++ {
		src := filepath.Join(base, "wt")
		makeTree(t, src)
		must(t, tr.Discard(src))
	}
	b := bins(t, dir)
	if len(b) != 5 {
		t.Fatalf("want 5 bins, got %v", b)
	}
	for _, bin := range b {
		if _, err := os.Stat(filepath.Join(bin, "wt", "node_modules")); err != nil {
			t.Fatalf("bin %s: %v", bin, err)
		}
	}
}

func TestDiscardMissingSource(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, ".trash")
	if err := New(dir).Discard(filepath.Join(base, "nope")); err == nil {
		t.Fatal("want error")
	}
	if b := bins(t, dir); len(b) != 0 {
		t.Fatalf("leftover bins: %v", b)
	}
}

func TestEmptyRemovesTrash(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, ".trash")
	tr := New(dir)
	src := filepath.Join(base, "wt")
	makeTree(t, src)
	must(t, tr.Discard(src))

	must(t, tr.Empty())

	if _, err := os.Lstat(dir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("trash dir still present: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		matches, err := filepath.Glob(dir + ".*")
		must(t, err)
		if len(matches) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("doomed dirs remain: %v", matches)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestEmptyMissingIsNoop(t *testing.T) {
	base := t.TempDir()
	must(t, New(filepath.Join(base, ".trash")).Empty())
	entries, err := os.ReadDir(base)
	must(t, err)
	if len(entries) != 0 {
		t.Fatalf("unexpected entries: %v", entries)
	}
}
