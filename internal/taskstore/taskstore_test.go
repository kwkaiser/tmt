package taskstore

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

func newTask(root, base, name string) tmt.Task {
	return tmt.Task{
		Dir:     filepath.Join(root, filepath.Base(base), name),
		Topic:   name,
		Base:    base,
		Branch:  "u/me/" + name + "-1",
		Session: name,
	}
}

func mustCreate(t *testing.T, s *Store, task tmt.Task) {
	t.Helper()
	if err := s.Create(t.Context(), task); err != nil {
		t.Fatal(err)
	}
}

func mkdirs(t *testing.T, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCreateListRoundTrip(t *testing.T) {
	root := t.TempDir()
	s := New(root)
	a := newTask(root, "/src/api", "alpha")
	b := newTask(root, "/src/web", "beta")
	mustCreate(t, s, b)
	mustCreate(t, s, a)
	mkdirs(t, filepath.Join(root, ".trash", "old"))
	writeFile(t, filepath.Join(root, ".trash", "old", metaFile))
	mkdirs(t, filepath.Join(root, "api", "nometa"))

	got, err := s.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if want := []tmt.Task{a, b}; !reflect.DeepEqual(got, want) {
		t.Fatalf("List = %+v; want %+v", got, want)
	}

	data, err := os.ReadFile(filepath.Join(a.Dir, metaFile))
	if err != nil {
		t.Fatal(err)
	}
	want := "topic=alpha\nbase=/src/api\nbranch=u/me/alpha-1\nsession=alpha\n"
	if string(data) != want {
		t.Fatalf(".tmt = %q; want %q", data, want)
	}

	if err := s.Create(t.Context(), a); err == nil {
		t.Fatal("expected error creating existing task")
	}
}

func TestListMissingRoot(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "missing"))
	got, err := s.List(t.Context())
	if err != nil || len(got) != 0 {
		t.Fatalf("List = %v, %v; want empty", got, err)
	}
}

func TestDecodeTolerant(t *testing.T) {
	got := decode([]byte("\nextra=1\ntopic=a\n\nbase=/b\nsession=s\nbranch=x=y\n"))
	want := tmt.Task{Topic: "a", Base: "/b", Branch: "x=y", Session: "s"}
	if got != want {
		t.Fatalf("decode = %+v; want %+v", got, want)
	}
}

func TestByTopic(t *testing.T) {
	root := t.TempDir()
	s := New(root)
	mustCreate(t, s, newTask(root, "/src/api", "solo"))
	mustCreate(t, s, newTask(root, "/src/api", "dup"))
	mustCreate(t, s, newTask(root, "/src/web", "dup"))

	_, err := s.ByTopic(t.Context(), "none")
	if !errors.Is(err, tmt.ErrTaskNotFound) {
		t.Fatalf("ByTopic(none) err = %v; want ErrTaskNotFound", err)
	}
	if want := "expected exactly one task named 'none', found 0"; err.Error() != want {
		t.Fatalf("ByTopic(none) err = %q; want %q", err, want)
	}

	got, err := s.ByTopic(t.Context(), "solo")
	if err != nil {
		t.Fatal(err)
	}
	if got.Dir != filepath.Join(root, "api", "solo") {
		t.Fatalf("ByTopic(solo).Dir = %s", got.Dir)
	}

	_, err = s.ByTopic(t.Context(), "dup")
	if err == nil || errors.Is(err, tmt.ErrTaskNotFound) {
		t.Fatalf("ByTopic(dup) err = %v; want ambiguity error", err)
	}
	if want := "expected exactly one task named 'dup', found 2"; err.Error() != want {
		t.Fatalf("ByTopic(dup) err = %q; want %q", err, want)
	}
}

func TestBySession(t *testing.T) {
	root := t.TempDir()
	s := New(root)
	a := newTask(root, "/src/api", "alpha")
	a.Session = "sess-a"
	mustCreate(t, s, a)
	mustCreate(t, s, newTask(root, "/src/api", "beta"))

	got, err := s.BySession(t.Context(), "sess-a")
	if err != nil {
		t.Fatal(err)
	}
	if got != a {
		t.Fatalf("BySession = %+v; want %+v", got, a)
	}
	for _, q := range []string{"", "missing"} {
		if _, err := s.BySession(t.Context(), q); !errors.Is(err, tmt.ErrTaskNotFound) {
			t.Fatalf("BySession(%q) err = %v; want ErrTaskNotFound", q, err)
		}
	}
}

func TestContaining(t *testing.T) {
	root := t.TempDir()
	s := New(root)
	a := newTask(root, "/src/api", "alpha")
	mustCreate(t, s, a)
	nested := filepath.Join(a.Dir, "repo", "pkg", "deep")
	mkdirs(t, nested)

	got, err := s.Containing(t.Context(), nested)
	if err != nil {
		t.Fatal(err)
	}
	if got != a {
		t.Fatalf("Containing = %+v; want %+v", got, a)
	}

	outside := t.TempDir()
	if _, err := s.Containing(t.Context(), outside); !errors.Is(err, tmt.ErrTaskNotFound) {
		t.Fatalf("Containing(outside) err = %v; want ErrTaskNotFound", err)
	}
}

func TestEntries(t *testing.T) {
	root := t.TempDir()
	s := New(root)
	base := t.TempDir()
	a := newTask(root, base, "alpha")
	mustCreate(t, s, a)
	mkdirs(t, filepath.Join(a.Dir, "wt"), filepath.Join(a.Dir, ".hidden"), filepath.Join(base, "lib"))
	if err := os.Symlink(filepath.Join(base, "lib"), filepath.Join(a.Dir, "lib")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(base, "gone"), filepath.Join(a.Dir, "dangling")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(a.Dir, "CLAUDE.md"))
	writeFile(t, filepath.Join(a.Dir, ".mcp.json"))

	got, err := s.Entries(t.Context(), a)
	if err != nil {
		t.Fatal(err)
	}
	want := []tmt.Entry{
		{Name: "dangling", Kind: tmt.EntryLink},
		{Name: "lib", Kind: tmt.EntryLink},
		{Name: "wt", Kind: tmt.EntryWorktree},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Entries = %+v; want %+v", got, want)
	}
}

func TestLinkUnlink(t *testing.T) {
	root := t.TempDir()
	s := New(root)
	base := t.TempDir()
	a := newTask(root, base, "alpha")
	mustCreate(t, s, a)

	if err := s.Link(t.Context(), a, "api"); err != nil {
		t.Fatal(err)
	}
	dest, err := os.Readlink(filepath.Join(a.Dir, "api"))
	if err != nil {
		t.Fatal(err)
	}
	if dest != filepath.Join(base, "api") {
		t.Fatalf("link target = %s", dest)
	}

	if err := s.Unlink(t.Context(), a, "api"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(a.Dir, "api")); !os.IsNotExist(err) {
		t.Fatalf("link still present: %v", err)
	}
	if err := s.Unlink(t.Context(), a, "api"); err != nil {
		t.Fatalf("Unlink missing = %v; want nil", err)
	}

	mkdirs(t, filepath.Join(a.Dir, "wt"))
	if err := s.Unlink(t.Context(), a, "wt"); err == nil || !strings.Contains(err.Error(), "directory") {
		t.Fatalf("Unlink dir err = %v; want refusal", err)
	}
	if _, err := os.Stat(filepath.Join(a.Dir, "wt")); err != nil {
		t.Fatalf("worktree removed: %v", err)
	}
}

func TestLinkMissing(t *testing.T) {
	root := t.TempDir()
	s := New(root)
	base := t.TempDir()
	other := t.TempDir()
	mkdirs(t,
		filepath.Join(base, "api", ".git"),
		filepath.Join(base, "web"),
		filepath.Join(base, "plain"),
		filepath.Join(base, "taken", ".git"),
		filepath.Join(base, "stale", ".git"),
		filepath.Join(base, ".hidden", ".git"),
		filepath.Join(other, "ext", ".git"),
	)
	writeFile(t, filepath.Join(base, "web", ".git"))
	writeFile(t, filepath.Join(base, "file"))
	if err := os.Symlink(filepath.Join(other, "ext"), filepath.Join(base, "ext")); err != nil {
		t.Fatal(err)
	}

	a := newTask(root, base, "alpha")
	mustCreate(t, s, a)
	mkdirs(t, filepath.Join(a.Dir, "taken"))
	if err := os.Symlink(filepath.Join(base, "nowhere"), filepath.Join(a.Dir, "stale")); err != nil {
		t.Fatal(err)
	}

	got, err := s.LinkMissing(t.Context(), a)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"api", "ext", "web"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("LinkMissing = %v; want %v", got, want)
	}
	dest, err := os.Readlink(filepath.Join(a.Dir, "api"))
	if err != nil || dest != filepath.Join(base, "api") {
		t.Fatalf("api link = %s, %v", dest, err)
	}

	again, err := s.LinkMissing(t.Context(), a)
	if err != nil || len(again) != 0 {
		t.Fatalf("second LinkMissing = %v, %v; want none", again, err)
	}
}

func TestRemove(t *testing.T) {
	root := t.TempDir()
	s := New(root)
	solo := newTask(root, "/src/api", "solo")
	x := newTask(root, "/src/web", "x")
	y := newTask(root, "/src/web", "y")
	for _, task := range []tmt.Task{solo, x, y} {
		mustCreate(t, s, task)
	}
	mkdirs(t, filepath.Join(solo.Dir, "wt", "sub"))

	if err := s.Remove(t.Context(), solo); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(root, "api")); !os.IsNotExist(err) {
		t.Fatalf("empty parent kept: %v", err)
	}

	if err := s.Remove(t.Context(), x); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(x.Dir); !os.IsNotExist(err) {
		t.Fatalf("task dir kept: %v", err)
	}
	if _, err := os.Stat(y.Dir); err != nil {
		t.Fatalf("sibling removed: %v", err)
	}
}
