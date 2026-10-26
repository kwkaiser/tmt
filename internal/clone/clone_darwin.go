package clone

import (
	"context"
	"errors"
	"io/fs"
	"os"

	"golang.org/x/sys/unix"
)

func clonePath(ctx context.Context, src, dst string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	err := unix.Clonefile(src, dst, unix.CLONE_NOFOLLOW)
	if err == nil {
		return nil
	}
	if errors.Is(err, unix.EEXIST) {
		return &fs.PathError{Op: "clonefile", Path: dst, Err: err}
	}
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	return copyPath(ctx, src, dst, cloneFile)
}

func cloneFile(src, dst string, perm fs.FileMode) error {
	err := unix.Clonefile(src, dst, unix.CLONE_NOFOLLOW)
	if err == nil {
		return nil
	}
	if errors.Is(err, unix.EEXIST) {
		return &fs.PathError{Op: "clonefile", Path: dst, Err: err}
	}
	if err := os.Remove(dst); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return copyFile(src, dst, perm)
}
