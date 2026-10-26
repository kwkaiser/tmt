package prompt

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

type Terminal struct {
	In  io.Reader
	Out io.Writer

	tty     *os.File
	mu      sync.Mutex
	br      *bufio.Reader
	pending chan line
}

type line struct {
	text string
	eof  bool
	err  error
}

func New() *Terminal {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return &Terminal{In: os.Stdin, Out: os.Stderr}
	}
	return &Terminal{In: tty, Out: tty, tty: tty}
}

func (t *Terminal) Close() error {
	if t.tty == nil {
		return nil
	}
	return t.tty.Close()
}

func (t *Terminal) Confirm(ctx context.Context, msg string) (bool, error) {
	if _, err := fmt.Fprintf(t.Out, "%s [y/N] ", msg); err != nil {
		return false, err
	}
	l, err := t.readLine(ctx)
	if err != nil || l.eof {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(l.text)) {
	case "y", "yes":
		return true, nil
	}
	return false, nil
}

func (t *Terminal) Input(ctx context.Context, prompt, placeholder string) (string, bool, error) {
	text := prompt
	if placeholder != "" {
		text += placeholder + " "
	}
	if _, err := io.WriteString(t.Out, text); err != nil {
		return "", false, err
	}
	l, err := t.readLine(ctx)
	if err != nil || l.eof {
		return "", false, err
	}
	return strings.TrimSpace(l.text), true, nil
}

func (t *Terminal) readLine(ctx context.Context) (line, error) {
	t.mu.Lock()
	if t.br == nil {
		t.br = bufio.NewReader(t.In)
	}
	if t.pending == nil {
		ch := make(chan line, 1)
		br := t.br
		go func() { ch <- read(br) }()
		t.pending = ch
	}
	ch := t.pending
	t.mu.Unlock()

	select {
	case l := <-ch:
		t.mu.Lock()
		t.pending = nil
		t.mu.Unlock()
		return l, l.err
	case <-ctx.Done():
		return line{}, ctx.Err()
	}
}

func read(br *bufio.Reader) line {
	s, err := br.ReadString('\n')
	switch {
	case err == io.EOF && s == "":
		return line{eof: true}
	case err != nil && err != io.EOF:
		return line{err: err}
	}
	return line{text: strings.TrimRight(s, "\r\n")}
}
