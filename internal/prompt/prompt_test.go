package prompt

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

var _ tmt.Prompter = (*Terminal)(nil)

func TestConfirm(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"y", "y\n", true},
		{"Y", "Y\n", true},
		{"yes", "yes\n", true},
		{"YES padded", "  YES \n", true},
		{"no newline", "y", true},
		{"crlf", "y\r\n", true},
		{"n", "n\n", false},
		{"no", "no\n", false},
		{"empty", "\n", false},
		{"other", "yep\n", false},
		{"eof", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			p := &Terminal{In: strings.NewReader(tc.in), Out: &out}
			got, err := p.Confirm(context.Background(), "close foo?")
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			if out.String() != "close foo? [y/N] " {
				t.Fatalf("out = %q", out.String())
			}
		})
	}
}

func TestInput(t *testing.T) {
	tests := []struct {
		name        string
		in          string
		placeholder string
		want        string
		ok          bool
		out         string
	}{
		{"trimmed", "  my-topic \n", "[A-Za-z0-9_-]+", "my-topic", true, "topic: [A-Za-z0-9_-]+ "},
		{"no newline", "abc", "", "abc", true, "topic: "},
		{"empty line", "\n", "", "", true, "topic: "},
		{"eof", "", "[A-Za-z0-9_-]+", "", false, "topic: [A-Za-z0-9_-]+ "},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			p := &Terminal{In: strings.NewReader(tc.in), Out: &out}
			got, ok, err := p.Input(context.Background(), "topic: ", tc.placeholder)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if got != tc.want || ok != tc.ok {
				t.Fatalf("got (%q, %v), want (%q, %v)", got, ok, tc.want, tc.ok)
			}
			if out.String() != tc.out {
				t.Fatalf("out = %q, want %q", out.String(), tc.out)
			}
		})
	}
}

func TestSequentialReads(t *testing.T) {
	p := &Terminal{In: strings.NewReader("y\nname\nn\n"), Out: io.Discard}
	ctx := context.Background()
	if ok, _ := p.Confirm(ctx, "a?"); !ok {
		t.Fatal("first confirm false")
	}
	if s, ok, _ := p.Input(ctx, "b: ", ""); s != "name" || !ok {
		t.Fatalf("input = %q %v", s, ok)
	}
	if ok, _ := p.Confirm(ctx, "c?"); ok {
		t.Fatal("last confirm true")
	}
}

func TestReadError(t *testing.T) {
	r, w := io.Pipe()
	boom := errors.New("boom")
	w.CloseWithError(boom)
	p := &Terminal{In: r, Out: io.Discard}
	if _, err := p.Confirm(context.Background(), "x?"); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

func TestCancel(t *testing.T) {
	tests := []struct {
		name string
		call func(context.Context, *Terminal) error
	}{
		{"confirm", func(ctx context.Context, p *Terminal) error {
			_, err := p.Confirm(ctx, "x?")
			return err
		}},
		{"input", func(ctx context.Context, p *Terminal) error {
			_, _, err := p.Input(ctx, "x: ", "")
			return err
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, w := io.Pipe()
			defer w.Close()
			p := &Terminal{In: r, Out: io.Discard}
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			go func() { done <- tc.call(ctx, p) }()
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("err = %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("did not return after cancel")
			}
		})
	}
}

func TestCancelThenRead(t *testing.T) {
	r, w := io.Pipe()
	p := &Terminal{In: r, Out: io.Discard}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.Confirm(ctx, "x?"); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	go w.Write([]byte("yes\n"))
	ok, err := p.Confirm(context.Background(), "x?")
	if err != nil || !ok {
		t.Fatalf("got %v %v", ok, err)
	}
}
