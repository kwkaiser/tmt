package tmux

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

const ownerOption = "@tmt_task"

type Tmux struct {
	Bin    string
	Getenv func(string) string
}

var _ tmt.Mux = (*Tmux)(nil)

func New() *Tmux {
	return &Tmux{Bin: "tmux", Getenv: os.Getenv}
}

func (t *Tmux) bin() string {
	if t.Bin == "" {
		return "tmux"
	}
	return t.Bin
}

func (t *Tmux) inside() bool {
	getenv := t.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	return getenv("TMUX") != ""
}

func (t *Tmux) run(ctx context.Context, args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, t.bin(), args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("tmux %s: %w: %s", args[0], err, msg)
		}
		return "", fmt.Errorf("tmux %s: %w", args[0], err)
	}
	return strings.TrimRight(stdout.String(), "\n"), nil
}

func (t *Tmux) interactive(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, t.bin(), args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tmux %s: %w", args[0], err)
	}
	return nil
}

func exited(err error) bool {
	var exit *exec.ExitError
	return errors.As(err, &exit)
}

func exact(name string) string {
	return "=" + name
}

func (t *Tmux) CurrentSession(ctx context.Context) (string, error) {
	if !t.inside() {
		return "", nil
	}
	if out, err := t.run(ctx, "display", "-p", "#S"); err == nil {
		return strings.TrimSpace(out), nil
	}
	return "", nil
}

func (t *Tmux) HasSession(ctx context.Context, name string) (bool, error) {
	_, err := t.run(ctx, "has-session", "-t", exact(name))
	if err == nil {
		return true, nil
	}
	if exited(err) {
		return false, nil
	}
	return false, err
}

func (t *Tmux) EnsureSession(ctx context.Context, s tmt.Session) error {
	has, err := t.HasSession(ctx, s.Name)
	if err != nil {
		return err
	}
	if has {
		owner, err := t.run(ctx, "show-options", "-t", exact(s.Name)+":", "-v", ownerOption)
		if err != nil && !exited(err) {
			return err
		}
		if strings.TrimSpace(owner) != s.Owner {
			return tmt.ErrForeignSession
		}
		return nil
	}
	pane, err := t.run(ctx, "new-session", "-d", "-P", "-F", "#{pane_id}", "-s", s.Name, "-n", s.RootName, "-c", s.Dir)
	if err != nil {
		return err
	}
	pane = strings.TrimSpace(pane)
	if _, err := t.run(ctx, "set-option", "-t", pane, ownerOption, s.Owner); err != nil {
		return err
	}
	if _, err := t.run(ctx, "send-keys", "-t", pane, "clear && "+s.Command, "Enter"); err != nil {
		return err
	}
	for _, w := range s.Windows {
		if _, err := t.run(ctx, "new-window", "-d", "-t", exact(s.Name)+":", "-n", w.Name, "-c", w.Dir); err != nil {
			return err
		}
	}
	return nil
}

func (t *Tmux) Attach(ctx context.Context, name string) error {
	if t.inside() {
		return t.interactive(ctx, "switch-client", "-t", exact(name))
	}
	return t.interactive(ctx, "attach-session", "-t", exact(name))
}

func (t *Tmux) KillSession(ctx context.Context, name string) error {
	current, err := t.CurrentSession(ctx)
	if err != nil {
		return err
	}
	if current == name {
		if _, err := t.run(ctx, "switch-client", "-l"); err != nil {
			_, _ = t.run(ctx, "switch-client", "-n")
		}
	}
	_, _ = t.run(ctx, "kill-session", "-t", exact(name))
	return nil
}

func (t *Tmux) FindWindow(ctx context.Context, session, name string) (tmt.WindowID, bool, error) {
	out, err := t.run(ctx, "list-windows", "-t", exact(session), "-F", "#{window_id} #{window_name}")
	if err != nil {
		if exited(err) {
			return "", false, nil
		}
		return "", false, err
	}
	id, ok := findWindow(out, name)
	return id, ok, nil
}

func findWindow(out, name string) (tmt.WindowID, bool) {
	for line := range strings.SplitSeq(out, "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[1] == name {
			return tmt.WindowID(f[0]), true
		}
	}
	return "", false
}

func (t *Tmux) NewWindow(ctx context.Context, session string, w tmt.Window) (tmt.WindowID, error) {
	out, err := t.run(ctx, "new-window", "-d", "-P", "-F", "#{window_id}", "-t", exact(session)+":", "-n", w.Name, "-c", w.Dir)
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(out)
	if id == "" {
		return "", errors.New("tmux new-window: no window id")
	}
	return tmt.WindowID(id), nil
}

func (t *Tmux) SelectWindow(ctx context.Context, id tmt.WindowID) error {
	_, err := t.run(ctx, "select-window", "-t", string(id))
	return err
}

func (t *Tmux) KillWindow(ctx context.Context, id tmt.WindowID) error {
	_, err := t.run(ctx, "kill-window", "-t", string(id))
	return err
}
