package cli

import (
	"maps"
	"testing"
)

func TestApplyBundled(t *testing.T) {
	env := map[string]string{"PATH": "/usr/bin", "TMT_COLOR_DIM": "1"}
	lookup := func(k string) (string, bool) {
		v, ok := env[k]
		return v, ok
	}
	setenv := func(k, v string) error {
		env[k] = v
		return nil
	}
	err := applyBundled("/nix/a/bin:/nix/b/bin", map[string]string{"TMT_COLOR_DIM": "2", "TMT_FZF_COLORS": "fg:1"},
		func(k string) string { return env[k] }, lookup, setenv)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"PATH":           "/nix/a/bin:/nix/b/bin:/usr/bin",
		"TMT_COLOR_DIM":  "1",
		"TMT_FZF_COLORS": "fg:1",
	}
	if !maps.Equal(env, want) {
		t.Fatalf("env = %v; want %v", env, want)
	}
}

func TestApplyBundledEmpty(t *testing.T) {
	called := false
	err := applyBundled("", nil, func(string) string { return "" }, func(string) (string, bool) { return "", false },
		func(string, string) error { called = true; return nil })
	if err != nil || called {
		t.Fatalf("err = %v, setenv called = %v", err, called)
	}
}
