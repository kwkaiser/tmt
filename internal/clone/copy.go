package clone

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

type fileFunc func(src, dst string, perm fs.FileMode) error

func copyPath(ctx context.Context, src, dst string, file fileFunc) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	mode := info.Mode()
	switch {
	case mode.IsDir():
		return copyDir(ctx, src, dst, mode.Perm(), file)
	case mode&fs.ModeSymlink != 0:
		return copySymlink(src, dst)
	case mode.IsRegular():
		return file(src, dst, mode.Perm())
	default:
		return fmt.Errorf("unsupported file type %s: %s", mode.Type(), src)
	}
}

func copyDir(ctx context.Context, src, dst string, perm fs.FileMode, file fileFunc) error {
	if err := os.Mkdir(dst, 0o700); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := copyPath(ctx, filepath.Join(src, e.Name()), filepath.Join(dst, e.Name()), file); err != nil {
			return err
		}
	}
	return os.Chmod(dst, perm)
}

func copySymlink(src, dst string) error {
	target, err := os.Readlink(src)
	if err != nil {
		return err
	}
	return os.Symlink(target, dst)
}

func copyFile(src, dst string, perm fs.FileMode) error {
	return writeFile(src, dst, perm, nil)
}

func writeFile(src, dst string, perm fs.FileMode, fast func(in, out *os.File) error) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if fast == nil || fast(in, out) != nil {
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(dst, perm)
}
