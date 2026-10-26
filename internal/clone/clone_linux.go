package clone

import (
	"context"
	"io/fs"
	"os"

	"golang.org/x/sys/unix"
)

func clonePath(ctx context.Context, src, dst string) error {
	return copyPath(ctx, src, dst, cloneFile)
}

func cloneFile(src, dst string, perm fs.FileMode) error {
	return writeFile(src, dst, perm, ficlone)
}

func ficlone(in, out *os.File) error {
	return unix.IoctlFileClone(int(out.Fd()), int(in.Fd()))
}
