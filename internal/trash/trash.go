package trash

import (
	"errors"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type Trash struct {
	dir string
}

func New(dir string) *Trash {
	return &Trash{dir: dir}
}

func (t *Trash) Discard(path string) error {
	if err := os.MkdirAll(t.dir, 0o755); err != nil {
		return err
	}
	bin, err := t.makeBin()
	if err != nil {
		return err
	}
	if err := os.Rename(path, filepath.Join(bin, filepath.Base(path))); err != nil {
		_ = os.Remove(bin)
		if crossDevice(err) {
			return fmt.Errorf("cannot trash %s: not on the same filesystem as %s", path, t.dir)
		}
		return err
	}
	return nil
}

func (t *Trash) Empty() error {
	if _, err := os.Lstat(t.dir); errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	doomed, err := t.claim()
	if err != nil {
		return err
	}
	return removeDetached(doomed)
}

func (t *Trash) makeBin() (string, error) {
	prefix := strconv.FormatInt(time.Now().Unix(), 10) + "-" + strconv.Itoa(os.Getpid()) + "-"
	for {
		bin := filepath.Join(t.dir, prefix+strconv.FormatUint(uint64(rand.Uint32()), 10))
		err := os.Mkdir(bin, 0o755)
		if err == nil {
			return bin, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", err
		}
	}
}

func (t *Trash) claim() (string, error) {
	doomed := t.dir + "." + strconv.Itoa(os.Getpid())
	for {
		_, err := os.Lstat(doomed)
		if errors.Is(err, fs.ErrNotExist) {
			if err := os.Rename(t.dir, doomed); err != nil {
				return "", err
			}
			return doomed, nil
		}
		if err != nil {
			return "", err
		}
		doomed = t.dir + "." + strconv.Itoa(os.Getpid()) + "-" + strconv.FormatUint(uint64(rand.Uint32()), 10)
	}
}
