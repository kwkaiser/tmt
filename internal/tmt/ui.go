package tmt

import "context"

const PickKey = "{key}"

type PickItem struct {
	Key     string
	Display string
}

type Pick struct {
	Label   string
	Header  string
	Preview []string
	Items   []PickItem
	Multi   bool
	Expect  []string
}

type Picked struct {
	Key   string
	Items []PickItem
}

type Picker interface {
	Pick(ctx context.Context, p Pick) (Picked, bool, error)
}

type Prompter interface {
	Confirm(ctx context.Context, msg string) (bool, error)
	Input(ctx context.Context, prompt, placeholder string) (string, bool, error)
}
