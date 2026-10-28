package herdr

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const workspaceListJSON = `{"id":"cli:workspace:list","result":{"type":"workspace_list","workspaces":[
{"active_tab_id":"w1:t9","focused":false,"label":"nixos-cfg","number":1,"workspace_id":"w1"},
{"active_tab_id":"wD:t1","focused":false,"label":"lila","number":2,"workspace_id":"wD"},
{"active_tab_id":"wF:t4","focused":true,"label":"tmt","number":3,"workspace_id":"wF"},
{"active_tab_id":"wG:t1","focused":false,"label":"lila","number":4,"workspace_id":"wG"}]}}`

const tabListJSON = `{"result":{"tabs":[
{"tab_id":"wF:t1","label":"1:claude"},
{"tab_id":"wF:t2","label":"api"},
{"tab_id":"wF:t3","label":"3:web"},
{"tab_id":"wF:t4","label":"12x"},
{"tab_id":"wF:t5","label":"notes:3:web"}]}}`

func decodeWorkspaces(t *testing.T) []workspace {
	t.Helper()
	var l workspaceList
	if err := json.Unmarshal([]byte(workspaceListJSON), &l); err != nil {
		t.Fatal(err)
	}
	return l.Result.Workspaces
}

func decodeTabs(t *testing.T) []tab {
	t.Helper()
	var l tabList
	if err := json.Unmarshal([]byte(tabListJSON), &l); err != nil {
		t.Fatal(err)
	}
	return l.Result.Tabs
}

func TestWorkspaceByLabel(t *testing.T) {
	ws := decodeWorkspaces(t)
	cases := map[string]string{
		"tmt":       "wF",
		"lila":      "wD",
		"nixos-cfg": "w1",
		"missing":   "",
	}
	for label, want := range cases {
		if got := workspaceByLabel(ws, label); got != want {
			t.Errorf("workspaceByLabel(%q) = %q; want %q", label, got, want)
		}
	}
}

func TestFocusedLabel(t *testing.T) {
	if got := focusedLabel(decodeWorkspaces(t)); got != "tmt" {
		t.Fatalf("focusedLabel = %q; want tmt", got)
	}
	if got := focusedLabel(nil); got != "" {
		t.Fatalf("focusedLabel(nil) = %q; want empty", got)
	}
}

func TestTabMatches(t *testing.T) {
	cases := []struct {
		label, name string
		want        bool
	}{
		{"claude", "claude", true},
		{"1:claude", "claude", true},
		{"12:claude", "claude", true},
		{"1:claude", "1:claude", true},
		{"x:claude", "claude", false},
		{"1:claude", "claud", false},
		{":claude", "claude", false},
		{"1:", "", true},
	}
	for _, c := range cases {
		if got := tabMatches(c.label, c.name); got != c.want {
			t.Errorf("tabMatches(%q, %q) = %v; want %v", c.label, c.name, got, c.want)
		}
	}
}

func TestTabByName(t *testing.T) {
	tabs := decodeTabs(t)
	cases := map[string]string{
		"claude":  "wF:t1",
		"api":     "wF:t2",
		"web":     "wF:t3",
		"3:web":   "wF:t3",
		"missing": "",
	}
	for name, want := range cases {
		if got := tabByName(tabs, name); got != want {
			t.Errorf("tabByName(%q) = %q; want %q", name, got, want)
		}
	}
}

func TestNextIndex(t *testing.T) {
	if got := nextIndex(decodeTabs(t)); got != 13 {
		t.Fatalf("nextIndex = %d; want 13", got)
	}
	if got := nextIndex(nil); got != 1 {
		t.Fatalf("nextIndex(nil) = %d; want 1", got)
	}
	if got := nextIndex([]tab{{Label: "api"}, {Label: "web"}}); got != 1 {
		t.Fatalf("nextIndex(unnumbered) = %d; want 1", got)
	}
}

func TestTabLabel(t *testing.T) {
	if got := tabLabel(4, "api"); got != "4:api" {
		t.Fatalf("tabLabel = %q", got)
	}
}

func TestDecodeCreated(t *testing.T) {
	var w workspaceCreated
	if err := json.Unmarshal([]byte(`{"result":{"root_pane":{"pane_id":"wZ:p1"},"tab":{"tab_id":"wZ:t1"}}}`), &w); err != nil {
		t.Fatal(err)
	}
	if w.Result.RootPane.ID != "wZ:p1" || w.Result.Tab.ID != "wZ:t1" {
		t.Fatalf("workspaceCreated = %+v", w)
	}
	var c tabCreated
	if err := json.Unmarshal([]byte(`{"result":{"tab":{"tab_id":"wZ:t2","label":"2:api"}}}`), &c); err != nil {
		t.Fatal(err)
	}
	if c.Result.Tab.ID != "wZ:t2" {
		t.Fatalf("tabCreated = %+v", c)
	}
}

func TestCurrentSessionOutsideHerdr(t *testing.T) {
	h := &Herdr{Getenv: func(string) string { return "" }}
	s, err := h.CurrentSession(t.Context())
	if err != nil || s != "" {
		t.Fatalf("CurrentSession = %q, %v; want empty", s, err)
	}
}

func TestSocketPath(t *testing.T) {
	tests := []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{"HERDR_SOCKET_PATH": "/run/h.sock", "HOME": "/home/u"}, "/run/h.sock"},
		{map[string]string{"XDG_CONFIG_HOME": "/cfg", "HOME": "/home/u"}, "/cfg/herdr/herdr.sock"},
		{map[string]string{"HOME": "/home/u"}, "/home/u/.config/herdr/herdr.sock"},
	}
	for _, tc := range tests {
		h := &Herdr{Getenv: func(k string) string { return tc.env[k] }}
		if got := h.socket(); got != tc.want {
			t.Errorf("socket(%v) = %q; want %q", tc.env, got, tc.want)
		}
	}
}

func serve(t *testing.T, reply string) (*Herdr, <-chan request) {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "herdr")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	path := filepath.Join(dir, "s")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	reqs := make(chan request, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		var r request
		if err := json.NewDecoder(conn).Decode(&r); err == nil {
			reqs <- r
		}
		_, _ = conn.Write([]byte(reply + "\n"))
	}()
	return &Herdr{Getenv: func(k string) string {
		if k == "HERDR_SOCKET_PATH" {
			return path
		}
		return ""
	}}, reqs
}

func TestCallWorkspaceList(t *testing.T) {
	h, reqs := serve(t, strings.ReplaceAll(workspaceListJSON, "\n", ""))
	ok, err := h.HasSession(t.Context(), "tmt")
	if err != nil || !ok {
		t.Fatalf("HasSession = %v, %v; want true", ok, err)
	}
	if r := <-reqs; r.Method != "workspace.list" {
		t.Fatalf("method = %q", r.Method)
	}
}

func TestCallParams(t *testing.T) {
	h, reqs := serve(t, `{"id":"tmt","result":{"type":"ok"}}`)
	if err := h.SelectWindow(t.Context(), "wF:t2"); err != nil {
		t.Fatal(err)
	}
	r := <-reqs
	params, _ := json.Marshal(r.Params)
	if r.Method != "tab.focus" || string(params) != `{"tab_id":"wF:t2"}` {
		t.Fatalf("request = %s %s", r.Method, params)
	}
}

func TestCallError(t *testing.T) {
	h, _ := serve(t, `{"id":"tmt","error":{"code":"tab_not_found","message":"tab x not found"}}`)
	err := h.SelectWindow(t.Context(), "x")
	if err == nil || !strings.Contains(err.Error(), "tab_not_found") {
		t.Fatalf("err = %v; want tab_not_found", err)
	}
}
