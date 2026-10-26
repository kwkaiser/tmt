package taskstore

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

const metaFile = ".tmt"

type Store struct {
	Root string
}

func New(root string) *Store {
	return &Store{Root: root}
}

var _ tmt.TaskStore = (*Store)(nil)

func (s *Store) List(ctx context.Context) ([]tmt.Task, error) {
	bases, err := visibleDirs(s.Root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var tasks []tmt.Task
	for _, b := range bases {
		names, err := visibleDirs(b)
		if err != nil {
			return nil, err
		}
		for _, d := range names {
			if !isRegular(filepath.Join(d, metaFile)) {
				continue
			}
			t, err := load(d)
			if err != nil {
				return nil, err
			}
			tasks = append(tasks, t)
		}
	}
	return tasks, nil
}

func (s *Store) ByTopic(ctx context.Context, topic string) (tmt.Task, error) {
	tasks, err := s.List(ctx)
	if err != nil {
		return tmt.Task{}, err
	}
	var matches []tmt.Task
	for _, t := range tasks {
		if filepath.Base(t.Dir) == topic {
			matches = append(matches, t)
		}
	}
	if len(matches) != 1 {
		return tmt.Task{}, topicError{topic: topic, found: len(matches)}
	}
	return matches[0], nil
}

type topicError struct {
	topic string
	found int
}

func (e topicError) Error() string {
	return fmt.Sprintf("expected exactly one task named '%s', found %d", e.topic, e.found)
}

func (e topicError) Is(target error) bool {
	return e.found == 0 && target == tmt.ErrTaskNotFound
}

func (s *Store) BySession(ctx context.Context, session string) (tmt.Task, error) {
	if session == "" {
		return tmt.Task{}, tmt.ErrTaskNotFound
	}
	tasks, err := s.List(ctx)
	if err != nil {
		return tmt.Task{}, err
	}
	for _, t := range tasks {
		if t.Session == session {
			return t, nil
		}
	}
	return tmt.Task{}, tmt.ErrTaskNotFound
}

func (s *Store) Containing(ctx context.Context, path string) (tmt.Task, error) {
	d, err := filepath.Abs(path)
	if err != nil {
		return tmt.Task{}, err
	}
	for d != string(filepath.Separator) {
		if isRegular(filepath.Join(d, metaFile)) {
			return load(d)
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}
	return tmt.Task{}, tmt.ErrTaskNotFound
}

func (s *Store) Create(ctx context.Context, t tmt.Task) error {
	if err := os.MkdirAll(t.Dir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(t.Dir, metaFile), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(encode(t)); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func (s *Store) Remove(ctx context.Context, t tmt.Task) error {
	if err := os.RemoveAll(t.Dir); err != nil {
		return err
	}
	_ = os.Remove(filepath.Dir(t.Dir))
	return nil
}

func (s *Store) Entries(ctx context.Context, t tmt.Task) ([]tmt.Entry, error) {
	children, err := os.ReadDir(t.Dir)
	if err != nil {
		return nil, err
	}
	var entries []tmt.Entry
	for _, c := range children {
		name := c.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		switch {
		case c.Type()&fs.ModeSymlink != 0:
			entries = append(entries, tmt.Entry{Name: name, Kind: tmt.EntryLink})
		case c.IsDir():
			entries = append(entries, tmt.Entry{Name: name, Kind: tmt.EntryWorktree})
		}
	}
	return entries, nil
}

func (s *Store) Link(ctx context.Context, t tmt.Task, repo string) error {
	return os.Symlink(filepath.Join(t.Base, repo), filepath.Join(t.Dir, repo))
}

func (s *Store) Unlink(ctx context.Context, t tmt.Task, repo string) error {
	p := filepath.Join(t.Dir, repo)
	fi, err := os.Lstat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if fi.IsDir() {
		return fmt.Errorf("%s is a directory, not a link", p)
	}
	return os.Remove(p)
}

func (s *Store) LinkMissing(ctx context.Context, t tmt.Task) ([]string, error) {
	repos, err := visibleDirs(t.Base)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var linked []string
	for _, r := range repos {
		if _, err := os.Stat(filepath.Join(r, ".git")); err != nil {
			continue
		}
		name := filepath.Base(r)
		target := filepath.Join(t.Dir, name)
		if _, err := os.Lstat(target); err == nil {
			continue
		}
		if err := os.Symlink(r, target); err != nil {
			return linked, err
		}
		linked = append(linked, name)
	}
	return linked, nil
}

func visibleDirs(dir string) ([]string, error) {
	children, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var dirs []string
	for _, c := range children {
		if strings.HasPrefix(c.Name(), ".") {
			continue
		}
		p := filepath.Join(dir, c.Name())
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			dirs = append(dirs, p)
		}
	}
	return dirs, nil
}

func isRegular(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular()
}

func load(dir string) (tmt.Task, error) {
	data, err := os.ReadFile(filepath.Join(dir, metaFile))
	if err != nil {
		return tmt.Task{}, err
	}
	t := decode(data)
	t.Dir = dir
	if t.Topic == "" {
		t.Topic = filepath.Base(dir)
	}
	return t, nil
}

func decode(data []byte) tmt.Task {
	var t tmt.Task
	seen := map[string]bool{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		key, value, ok := strings.Cut(sc.Text(), "=")
		if !ok || seen[key] {
			continue
		}
		seen[key] = true
		switch key {
		case "topic":
			t.Topic = value
		case "base":
			t.Base = value
		case "branch":
			t.Branch = value
		case "session":
			t.Session = value
		}
	}
	return t
}

func encode(t tmt.Task) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "topic=%s\n", t.Topic)
	fmt.Fprintf(&b, "base=%s\n", t.Base)
	fmt.Fprintf(&b, "branch=%s\n", t.Branch)
	fmt.Fprintf(&b, "session=%s\n", t.Session)
	return b.Bytes()
}
