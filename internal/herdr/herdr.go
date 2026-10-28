package herdr

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

const defaultSortBin = "herdr-sort-tabs"

type Herdr struct {
	SortBin string
	Getenv  func(string) string
}

var _ tmt.Mux = (*Herdr)(nil)

func New() *Herdr {
	return &Herdr{SortBin: defaultSortBin, Getenv: os.Getenv}
}

type workspace struct {
	ID      string `json:"workspace_id"`
	Label   string `json:"label"`
	Focused bool   `json:"focused"`
}

type tab struct {
	ID    string `json:"tab_id"`
	Label string `json:"label"`
}

type pane struct {
	ID string `json:"pane_id"`
}

type workspaceList struct {
	Result struct {
		Workspaces []workspace `json:"workspaces"`
	} `json:"result"`
}

type tabList struct {
	Result struct {
		Tabs []tab `json:"tabs"`
	} `json:"result"`
}

type tabCreated struct {
	Result struct {
		Tab tab `json:"tab"`
	} `json:"result"`
}

type workspaceCreated struct {
	Result struct {
		RootPane pane `json:"root_pane"`
		Tab      tab  `json:"tab"`
	} `json:"result"`
}

func (h *Herdr) getenv(key string) string {
	if h.Getenv == nil {
		return os.Getenv(key)
	}
	return h.Getenv(key)
}

func (h *Herdr) socket() string {
	if p := h.getenv("HERDR_SOCKET_PATH"); p != "" {
		return p
	}
	dir := h.getenv("XDG_CONFIG_HOME")
	if dir == "" {
		dir = filepath.Join(h.getenv("HOME"), ".config")
	}
	return filepath.Join(dir, "herdr", "herdr.sock")
}

type request struct {
	ID     string `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params"`
}

type errorResponse struct {
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (h *Herdr) call(ctx context.Context, v any, method string, params any) error {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", h.socket())
	if err != nil {
		return fmt.Errorf("herdr %s: %w", method, err)
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	defer stop()

	if err := json.NewEncoder(conn).Encode(request{ID: "tmt", Method: method, Params: params}); err != nil {
		return fmt.Errorf("herdr %s: %w", method, err)
	}
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return fmt.Errorf("herdr %s: %w", method, err)
	}
	var e errorResponse
	if err := json.Unmarshal(line, &e); err != nil {
		return fmt.Errorf("herdr %s: decode: %w", method, err)
	}
	if e.Error != nil {
		return fmt.Errorf("herdr %s: %s: %s", method, e.Error.Code, e.Error.Message)
	}
	if v == nil {
		return nil
	}
	if err := json.Unmarshal(line, v); err != nil {
		return fmt.Errorf("herdr %s: decode: %w", method, err)
	}
	return nil
}

func (h *Herdr) workspaces(ctx context.Context) ([]workspace, error) {
	var l workspaceList
	if err := h.call(ctx, &l, "workspace.list", struct{}{}); err != nil {
		return nil, err
	}
	return l.Result.Workspaces, nil
}

func (h *Herdr) tabs(ctx context.Context, ws string) ([]tab, error) {
	var l tabList
	if err := h.call(ctx, &l, "tab.list", map[string]string{"workspace_id": ws}); err != nil {
		return nil, err
	}
	return l.Result.Tabs, nil
}

func (h *Herdr) workspaceID(ctx context.Context, label string) (string, error) {
	ws, err := h.workspaces(ctx)
	if err != nil {
		return "", err
	}
	return workspaceByLabel(ws, label), nil
}

func workspaceByLabel(ws []workspace, label string) string {
	for _, w := range ws {
		if w.Label == label {
			return w.ID
		}
	}
	return ""
}

func focusedLabel(ws []workspace) string {
	for _, w := range ws {
		if w.Focused {
			return w.Label
		}
	}
	return ""
}

var (
	indexPrefix = regexp.MustCompile(`^[0-9]+:`)
	leadingNum  = regexp.MustCompile(`^[0-9]+`)
)

func tabMatches(label, name string) bool {
	if label == name {
		return true
	}
	return indexPrefix.MatchString(label) && indexPrefix.ReplaceAllString(label, "") == name
}

func tabByName(tabs []tab, name string) string {
	for _, t := range tabs {
		if tabMatches(t.Label, name) {
			return t.ID
		}
	}
	return ""
}

func nextIndex(tabs []tab) int {
	top := 0
	for _, t := range tabs {
		m := leadingNum.FindString(t.Label)
		if m == "" {
			continue
		}
		if n, err := strconv.Atoi(m); err == nil && n > top {
			top = n
		}
	}
	return top + 1
}

func tabLabel(index int, name string) string {
	return strconv.Itoa(index) + ":" + name
}

func (h *Herdr) CurrentSession(ctx context.Context) (string, error) {
	if h.getenv("HERDR_ENV") != "1" {
		return "", nil
	}
	ws, err := h.workspaces(ctx)
	if err != nil {
		return "", err
	}
	return focusedLabel(ws), nil
}

func (h *Herdr) HasSession(ctx context.Context, name string) (bool, error) {
	id, err := h.workspaceID(ctx, name)
	if err != nil {
		return false, err
	}
	return id != "", nil
}

func (h *Herdr) Sessions(ctx context.Context) ([]string, error) {
	ws, err := h.workspaces(ctx)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(ws))
	for i, w := range ws {
		names[i] = w.Label
	}
	return names, nil
}

func (h *Herdr) EnsureSession(ctx context.Context, s tmt.Session) error {
	has, err := h.HasSession(ctx, s.Name)
	if err != nil || has {
		return err
	}
	var c workspaceCreated
	if err := h.call(ctx, &c, "workspace.create", map[string]any{"cwd": s.Dir, "label": s.Name, "focus": false}); err != nil {
		return err
	}
	if err := h.call(ctx, nil, "tab.rename", map[string]string{"tab_id": c.Result.Tab.ID, "label": tabLabel(1, s.RootName)}); err != nil {
		return err
	}
	if err := h.call(ctx, nil, "pane.send_input", map[string]any{
		"pane_id": c.Result.RootPane.ID,
		"text":    "clear && " + s.Command,
		"keys":    []string{"Enter"},
	}); err != nil {
		return err
	}
	for _, w := range s.Windows {
		if _, err := h.NewWindow(ctx, s.Name, w); err != nil {
			return err
		}
	}
	return nil
}

func (h *Herdr) Attach(ctx context.Context, name string) error {
	id, err := h.workspaceID(ctx, name)
	if err != nil {
		return err
	}
	if id == "" {
		return fmt.Errorf("herdr workspace %q not found", name)
	}
	return h.call(ctx, nil, "workspace.focus", map[string]string{"workspace_id": id})
}

func (h *Herdr) KillSession(ctx context.Context, name string) error {
	if id, err := h.workspaceID(ctx, name); err == nil && id != "" {
		_ = h.call(ctx, nil, "workspace.close", map[string]string{"workspace_id": id})
	}
	return nil
}

func (h *Herdr) FindWindow(ctx context.Context, session, name string) (tmt.WindowID, bool, error) {
	ws, err := h.workspaceID(ctx, session)
	if err != nil || ws == "" {
		return "", false, err
	}
	tabs, err := h.tabs(ctx, ws)
	if err != nil {
		return "", false, err
	}
	id := tabByName(tabs, name)
	return tmt.WindowID(id), id != "", nil
}

func (h *Herdr) NewWindow(ctx context.Context, session string, w tmt.Window) (tmt.WindowID, error) {
	ws, err := h.workspaceID(ctx, session)
	if err != nil {
		return "", err
	}
	if ws == "" {
		return "", fmt.Errorf("herdr workspace %q not found", session)
	}
	tabs, err := h.tabs(ctx, ws)
	if err != nil {
		return "", err
	}
	var c tabCreated
	if err := h.call(ctx, &c, "tab.create", map[string]any{
		"workspace_id": ws,
		"cwd":          w.Dir,
		"label":        tabLabel(nextIndex(tabs), w.Name),
		"focus":        false,
	}); err != nil {
		return "", err
	}
	if c.Result.Tab.ID == "" {
		return "", errors.New("herdr tab create: no tab id")
	}
	h.sortTabs(ctx, ws)
	return tmt.WindowID(c.Result.Tab.ID), nil
}

func (h *Herdr) sortTabs(ctx context.Context, ws string) {
	bin := h.SortBin
	if bin == "" {
		bin = defaultSortBin
	}
	path, err := exec.LookPath(bin)
	if err != nil {
		return
	}
	_ = exec.CommandContext(ctx, path, ws).Run()
}

func (h *Herdr) SelectWindow(ctx context.Context, id tmt.WindowID) error {
	return h.call(ctx, nil, "tab.focus", map[string]string{"tab_id": string(id)})
}

func (h *Herdr) KillWindow(ctx context.Context, id tmt.WindowID) error {
	return h.call(ctx, nil, "tab.close", map[string]string{"tab_id": string(id)})
}
