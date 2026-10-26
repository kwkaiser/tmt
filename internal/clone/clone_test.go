package clone

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

type entry struct {
	mode    fs.FileMode
	content string
	link    string
}

func buildTree(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "src")
	must(t, os.MkdirAll(filepath.Join(root, "a", "b"), 0o755))
	must(t, os.Mkdir(filepath.Join(root, "empty"), 0o750))
	must(t, os.WriteFile(filepath.Join(root, "top.txt"), []byte("top"), 0o644))
	must(t, os.WriteFile(filepath.Join(root, "a", "b", "deep.txt"), []byte("deep"), 0o600))
	must(t, os.WriteFile(filepath.Join(root, "a", "run.sh"), []byte("#!/bin/sh\n"), 0o755))
	must(t, os.Chmod(filepath.Join(root, "a", "run.sh"), 0o755))
	must(t, os.Symlink("b/deep.txt", filepath.Join(root, "a", "link")))
	must(t, os.Symlink("/nonexistent/target", filepath.Join(root, "dangling")))
	return root
}

func expectedTree() map[string]entry {
	return map[string]entry{
		".":            {mode: fs.ModeDir | 0o755},
		"a":            {mode: fs.ModeDir | 0o755},
		"a/b":          {mode: fs.ModeDir | 0o755},
		"empty":        {mode: fs.ModeDir | 0o750},
		"top.txt":      {mode: 0o644, content: "top"},
		"a/b/deep.txt": {mode: 0o600, content: "deep"},
		"a/run.sh":     {mode: 0o755, content: "#!/bin/sh\n"},
		"a/link":       {mode: fs.ModeSymlink, link: "b/deep.txt"},
		"dangling":     {mode: fs.ModeSymlink, link: "/nonexistent/target"},
	}
}

func snapshot(t *testing.T, root string) map[string]entry {
	t.Helper()
	got := map[string]entry{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		info, err := os.Lstat(p)
		if err != nil {
			return err
		}
		got[filepath.ToSlash(rel)] = describe(t, p, info)
		return nil
	})
	must(t, err)
	return got
}

func describe(t *testing.T, p string, info fs.FileInfo) entry {
	t.Helper()
	mode := info.Mode()
	switch {
	case mode&fs.ModeSymlink != 0:
		target, err := os.Readlink(p)
		must(t, err)
		return entry{mode: fs.ModeSymlink, link: target}
	case mode.IsDir():
		return entry{mode: fs.ModeDir | mode.Perm()}
	default:
		data, err := os.ReadFile(p)
		must(t, err)
		return entry{mode: mode.Perm(), content: string(data)}
	}
}

func assertTree(t *testing.T, root string, want map[string]entry) {
	t.Helper()
	got := snapshot(t, root)
	for k, w := range want {
		g, ok := got[k]
		if !ok {
			t.Errorf("missing %s", k)
			continue
		}
		if g != w {
			t.Errorf("%s: got %+v, want %+v", k, g, w)
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			t.Errorf("unexpected %s", k)
		}
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

type cloneFunc func(ctx context.Context, src, dst string) error

func implementations() map[string]cloneFunc {
	return map[string]cloneFunc{
		"clone": New().Clone,
		"fallback": func(ctx context.Context, src, dst string) error {
			return copyPath(ctx, src, dst, copyFile)
		},
	}
}

func TestCloneTree(t *testing.T) {
	for name, fn := range implementations() {
		t.Run(name, func(t *testing.T) {
			src := buildTree(t)
			dst := filepath.Join(t.TempDir(), "dst")
			must(t, fn(context.Background(), src, dst))
			assertTree(t, dst, expectedTree())
			assertTree(t, src, expectedTree())
		})
	}
}

func TestCloneFile(t *testing.T) {
	for name, fn := range implementations() {
		t.Run(name, func(t *testing.T) {
			src := filepath.Join(buildTree(t), "a", "run.sh")
			dst := filepath.Join(t.TempDir(), "run.sh")
			must(t, fn(context.Background(), src, dst))
			info, err := os.Lstat(dst)
			must(t, err)
			if got, want := describe(t, dst, info), (entry{mode: 0o755, content: "#!/bin/sh\n"}); got != want {
				t.Fatalf("got %+v, want %+v", got, want)
			}
		})
	}
}

func TestCloneSymlink(t *testing.T) {
	for name, fn := range implementations() {
		t.Run(name, func(t *testing.T) {
			src := filepath.Join(buildTree(t), "a", "link")
			dst := filepath.Join(t.TempDir(), "link")
			must(t, fn(context.Background(), src, dst))
			info, err := os.Lstat(dst)
			must(t, err)
			if got, want := describe(t, dst, info), (entry{mode: fs.ModeSymlink, link: "b/deep.txt"}); got != want {
				t.Fatalf("got %+v, want %+v", got, want)
			}
		})
	}
}

func TestCloneExistingDst(t *testing.T) {
	for name, fn := range implementations() {
		t.Run(name, func(t *testing.T) {
			src := buildTree(t)
			dst := t.TempDir()
			must(t, os.WriteFile(filepath.Join(dst, "keep"), []byte("keep"), 0o644))
			if err := fn(context.Background(), src, dst); err == nil {
				t.Fatal("expected error cloning onto existing dst")
			}
			if _, err := os.Stat(filepath.Join(dst, "keep")); err != nil {
				t.Fatalf("existing dst was modified: %v", err)
			}
		})
	}
}

func TestCloneCanceled(t *testing.T) {
	for name, fn := range implementations() {
		t.Run(name, func(t *testing.T) {
			src := buildTree(t)
			dst := filepath.Join(t.TempDir(), "dst")
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := fn(ctx, src, dst); !errors.Is(err, context.Canceled) {
				t.Fatalf("got %v, want context.Canceled", err)
			}
		})
	}
}

func TestFallbackStopsOnCancel(t *testing.T) {
	src := buildTree(t)
	dst := filepath.Join(t.TempDir(), "dst")
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err := copyPath(ctx, src, dst, func(s, d string, perm fs.FileMode) error {
		calls++
		cancel()
		return copyFile(s, d, perm)
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
	if calls != 1 {
		t.Fatalf("file copied %d times after cancel, want 1", calls)
	}
}

func TestPlatformFileClone(t *testing.T) {
	src := filepath.Join(buildTree(t), "a", "run.sh")
	dst := filepath.Join(t.TempDir(), "run.sh")
	must(t, cloneFile(src, dst, 0o755))
	info, err := os.Lstat(dst)
	must(t, err)
	if got, want := describe(t, dst, info), (entry{mode: 0o755, content: "#!/bin/sh\n"}); got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestFallbackFromPlatformFile(t *testing.T) {
	src := buildTree(t)
	dst := filepath.Join(t.TempDir(), "dst")
	must(t, copyPath(context.Background(), src, dst, cloneFile))
	assertTree(t, dst, expectedTree())
}
