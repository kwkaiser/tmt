package clone

import (
	"context"

	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

type cloner struct{}

func New() tmt.Cloner {
	return cloner{}
}

func (cloner) Clone(ctx context.Context, src, dst string) error {
	return clonePath(ctx, src, dst)
}
