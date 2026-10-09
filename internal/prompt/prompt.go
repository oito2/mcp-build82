// Copyright (C) 2026  OITO2
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

// Package prompt reads answers to the interactive questions of the CLI subcommands (install,
// uninstall, self-update). A read gives up as soon as its context ends — a first Ctrl-C — instead
// of waiting for an answer that will never come.
package prompt

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ErrInterrupted is returned when the context ends while a question waits for its answer. It wraps
// the context's error, so errors.Is(err, context.Canceled) also holds for a Ctrl-C.
var ErrInterrupted = errors.New("interrupted")

// interrupted wraps the error of the ended context `ctx` with ErrInterrupted.
func interrupted(ctx context.Context) error {
	return fmt.Errorf("%w: %w", ErrInterrupted, ctx.Err())
}

// ReadLine reads one line from `in`, including its newline, waiting at most until `ctx` ends. eof
// is true when the input ended or failed before a newline was read; `line` then holds what was
// read so far. It returns an error wrapping ErrInterrupted when `ctx` ends first; the read itself
// keeps waiting in the background, which is harmless as the process is about to exit.
func ReadLine(ctx context.Context, in *bufio.Reader) (line string, eof bool, err error) {
	if ctx.Err() != nil {
		return "", false, interrupted(ctx)
	}
	type result struct {
		line string
		err  error
	}
	read := make(chan result, 1)
	go func() {
		line, err := in.ReadString('\n')
		read <- result{line, err}
	}()
	select {
	case r := <-read:
		return r.line, r.err != nil, nil
	case <-ctx.Done():
		return "", false, interrupted(ctx)
	}
}

// Confirm prints `question` to `out` and reports whether the line read from `in` is "y", ignoring
// case and surrounding spaces. Any other answer, including an empty one or a closed input, is a
// "no". It returns an error wrapping ErrInterrupted when `ctx` ends before the answer, after
// ending the question's line on `out`.
func Confirm(ctx context.Context, in *bufio.Reader, out io.Writer, question string) (bool, error) {
	fmt.Fprint(out, question)
	line, _, err := ReadLine(ctx, in)
	if err != nil {
		fmt.Fprintln(out)
		return false, err
	}
	return strings.EqualFold(strings.TrimSpace(line), "y"), nil
}
