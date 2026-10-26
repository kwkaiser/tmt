package zoxide

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func fake(t *testing.T, script string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "zoxide")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

func TestQuery(t *testing.T) {
	bin := fake(t, `[ "$1 $2 $3" = "query -- proj" ] || exit 2
echo "  /home/u/proj  "
`)
	z := &Index{Bin: bin}
	dir, err := z.Query(t.Context(), "proj")
	if err != nil || dir != "/home/u/proj" {
		t.Fatalf("Query = %q, %v; want /home/u/proj", dir, err)
	}
}

func TestQueryDashArg(t *testing.T) {
	bin := fake(t, `[ "$2" = "--" ] && [ "$3" = "-l" ] || exit 2
echo /dash
`)
	z := &Index{Bin: bin}
	dir, err := z.Query(t.Context(), "-l")
	if err != nil || dir != "/dash" {
		t.Fatalf("Query = %q, %v; want /dash", dir, err)
	}
}

func TestQueryNoMatch(t *testing.T) {
	bin := fake(t, "echo 'zoxide: no match found' >&2\nexit 1\n")
	z := &Index{Bin: bin}
	dir, err := z.Query(t.Context(), "nothing")
	if err != nil || dir != "" {
		t.Fatalf("Query = %q, %v; want empty, nil", dir, err)
	}
}

func TestQueryMissingBinary(t *testing.T) {
	z := &Index{Bin: filepath.Join(t.TempDir(), "missing")}
	if _, err := z.Query(t.Context(), "proj"); err == nil {
		t.Fatal("expected error for missing binary")
	}
}

func TestList(t *testing.T) {
	bin := fake(t, `[ "$1 $2" = "query -l" ] || exit 2
printf '/a\n\n/b c\n/d\n'
`)
	z := &Index{Bin: bin}
	dirs, err := z.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"/a", "/b c", "/d"}; !slices.Equal(dirs, want) {
		t.Fatalf("List = %q; want %q", dirs, want)
	}
}

func TestListFailure(t *testing.T) {
	z := &Index{Bin: fake(t, "exit 1\n")}
	if _, err := z.List(t.Context()); err == nil {
		t.Fatal("expected error on non-zero exit")
	}
}

func TestListMissingBinary(t *testing.T) {
	z := &Index{Bin: filepath.Join(t.TempDir(), "missing")}
	if _, err := z.List(t.Context()); err == nil {
		t.Fatal("expected error for missing binary")
	}
}

func TestNewDefaultBin(t *testing.T) {
	if b := New().bin(); b != "zoxide" {
		t.Fatalf("bin = %q; want zoxide", b)
	}
	if b := (&Index{}).bin(); b != "zoxide" {
		t.Fatalf("empty bin = %q; want zoxide", b)
	}
}
