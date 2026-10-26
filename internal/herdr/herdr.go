package herdr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

const (
	defaultBin     = "herdr"
	defaultSortBin = "herdr-sort-tabs"
)

type Herdr struct {
	Bin     string
	SortBin string
	Getenv  func(string) string
}

var _ tmt.Mux = (*Herdr)(nil)

func New() *Herdr {
	bin := os.Getenv("HERDR_BIN_PATH")
	if bin == "" {
		bin = defaultBin
	}
	return &Herdr{Bin: bin, SortBin: defaultSortBin, Getenv: os.Getenv}
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

func (h *Herdr) bin() string {
	if h.Bin == "" {
		return defaultBin
	}
	return h.Bin
}

func (h *Herdr) getenv(key string) string {
	if h.Getenv == nil {
		return os.Getenv(key)
	}
	return h.Getenv(key)
}

func (h *Herdr) run(ctx context.Context, args ...string) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, h.bin(), args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		name := strings.Join(args[:min(2, len(args))], " ")
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("herdr %s: %w: %s", name, err, msg)
		}
		return nil, fmt.Errorf("herdr %s: %w", name, err)
	}
	return stdout.Bytes(), nil
}

func (h *Herdr) runJSON(ctx context.Context, v any, args ...string) error {
	out, err := h.run(ctx, args...)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(out, v); err != nil {
		return fmt.Errorf("herdr %s: decode: %w", strings.Join(args[:min(2, len(args))], " "), err)
	}
	return nil
}

func (h *Herdr) workspaces(ctx context.Context) ([]workspace, error) {
	var l workspaceList
	if err := h.runJSON(ctx, &l, "workspace", "list"); err != nil {
		return nil, err
	}
	return l.Result.Workspaces, nil
}

func (h *Herdr) tabs(ctx context.Context, ws string) ([]tab, error) {
	var l tabList
	if err := h.runJSON(ctx, &l, "tab", "list", "--workspace", ws); err != nil {
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

func (h *Herdr) EnsureSession(ctx context.Context, s tmt.Session) error {
	has, err := h.HasSession(ctx, s.Name)
	if err != nil || has {
		return err
	}
	var c workspaceCreated
	if err := h.runJSON(ctx, &c, "workspace", "create", "--cwd", s.Dir, "--label", s.Name, "--no-focus"); err != nil {
		return err
	}
	if _, err := h.run(ctx, "tab", "rename", c.Result.Tab.ID, tabLabel(1, s.RootName)); err != nil {
		return err
	}
	if _, err := h.run(ctx, "pane", "run", c.Result.RootPane.ID, "clear && "+s.Command); err != nil {
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
	_, err = h.run(ctx, "workspace", "focus", id)
	return err
}

func (h *Herdr) KillSession(ctx context.Context, name string) error {
	if id, err := h.workspaceID(ctx, name); err == nil && id != "" {
		_, _ = h.run(ctx, "workspace", "close", id)
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
	if err := h.runJSON(ctx, &c, "tab", "create", "--workspace", ws, "--cwd", w.Dir, "--label", tabLabel(nextIndex(tabs), w.Name), "--no-focus"); err != nil {
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
	_, err := h.run(ctx, "tab", "focus", string(id))
	return err
}

func (h *Herdr) KillWindow(ctx context.Context, id tmt.WindowID) error {
	_, err := h.run(ctx, "tab", "close", string(id))
	return err
}
