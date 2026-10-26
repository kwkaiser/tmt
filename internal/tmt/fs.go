package tmt

import "context"

type Cloner interface {
	Clone(ctx context.Context, src, dst string) error
}

type Trash interface {
	Discard(path string) error
	Empty() error
}

type DirIndex interface {
	Query(ctx context.Context, q string) (string, error)
	List(ctx context.Context) ([]string, error)
}
