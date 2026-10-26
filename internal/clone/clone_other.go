//go:build !darwin && !linux

package clone

import (
	"context"
	"io/fs"
)

func clonePath(ctx context.Context, src, dst string) error {
	return copyPath(ctx, src, dst, cloneFile)
}

func cloneFile(src, dst string, perm fs.FileMode) error {
	return copyFile(src, dst, perm)
}
